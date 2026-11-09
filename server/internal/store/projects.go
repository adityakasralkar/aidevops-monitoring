package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// MaxProjectsListed caps how many projects one list call returns.
const MaxProjectsListed = 500

// CreateProject inserts a new project and sets p.ID.
// It returns model.ErrProjectExists if the owner already has a project
// with the same name in the same environment.
func (s *Store) CreateProject(ctx context.Context, p *model.Project) error {
	res, err := s.Collection(CollApplications).InsertOne(ctx, p)
	if isDuplicateOn(err, indexOwnerProjectEnv) {
		return model.ErrProjectExists
	}
	if err != nil {
		return fmt.Errorf("insert project: %w", err)
	}
	p.ID = res.InsertedID.(bson.ObjectID)
	return nil
}

// ProjectsByOwner returns the owner's projects, newest first, up to
// MaxProjectsListed. The API key hash is left out of the result.
func (s *Store) ProjectsByOwner(ctx context.Context, ownerID bson.ObjectID) ([]model.Project, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(MaxProjectsListed).
		SetProjection(bson.D{{Key: "api_key_hash", Value: 0}})

	cur, err := s.Collection(CollApplications).Find(ctx, bson.D{{Key: "owner_id", Value: ownerID}}, opts)
	if err != nil {
		return nil, fmt.Errorf("find projects: %w", err)
	}
	projects := []model.Project{}
	if err := cur.All(ctx, &projects); err != nil {
		return nil, fmt.Errorf("read projects: %w", err)
	}
	return projects, nil
}

// ProjectByAPIKeyHash finds the project that owns an API key.
// It returns model.ErrNotFound if no project has this key.
func (s *Store) ProjectByAPIKeyHash(ctx context.Context, keyHash string) (model.Project, error) {
	var p model.Project
	err := s.Collection(CollApplications).FindOne(ctx, bson.D{{Key: "api_key_hash", Value: keyHash}}).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.Project{}, model.ErrNotFound
	}
	if err != nil {
		return model.Project{}, fmt.Errorf("find project by key: %w", err)
	}
	return p, nil
}

// ownedProject is the filter for one project of one owner. Using both ids
// means another user's project is simply "not found", never "forbidden",
// so its existence is not revealed.
func ownedProject(ownerID, projectID bson.ObjectID) bson.D {
	return bson.D{{Key: "_id", Value: projectID}, {Key: "owner_id", Value: ownerID}}
}

// ProjectByID returns one of the owner's projects, without the key hash.
// It returns model.ErrNotFound if it does not exist or belongs to someone else.
func (s *Store) ProjectByID(ctx context.Context, ownerID, projectID bson.ObjectID) (model.Project, error) {
	var p model.Project
	opts := options.FindOne().SetProjection(bson.D{{Key: "api_key_hash", Value: 0}})
	err := s.Collection(CollApplications).FindOne(ctx, ownedProject(ownerID, projectID), opts).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.Project{}, model.ErrNotFound
	}
	if err != nil {
		return model.Project{}, fmt.Errorf("find project: %w", err)
	}
	return p, nil
}

// RenameProject changes a project's name. It returns model.ErrNotFound if
// the owner has no such project, and model.ErrProjectExists if the owner
// already has another project with that name in the same environment.
func (s *Store) RenameProject(ctx context.Context, ownerID, projectID bson.ObjectID, name string) error {
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "project_name", Value: name}}}}
	res, err := s.Collection(CollApplications).UpdateOne(ctx, ownedProject(ownerID, projectID), update)
	if isDuplicateOn(err, indexOwnerProjectEnv) {
		return model.ErrProjectExists
	}
	if err != nil {
		return fmt.Errorf("rename project: %w", err)
	}
	if res.MatchedCount == 0 {
		return model.ErrNotFound
	}
	return nil
}

// RotateAPIKey replaces a project's API key. The old key stops working at
// once. It returns model.ErrNotFound if the owner has no such project.
func (s *Store) RotateAPIKey(ctx context.Context, ownerID, projectID bson.ObjectID, keyHash, keyPrefix string, now time.Time) error {
	update := bson.D{{Key: "$set", Value: bson.D{
		{Key: "api_key_hash", Value: keyHash},
		{Key: "key_prefix", Value: keyPrefix},
		{Key: "key_created_at", Value: now},
	}}, {Key: "$unset", Value: bson.D{{Key: "last_used_at", Value: ""}}}}
	res, err := s.Collection(CollApplications).UpdateOne(ctx, ownedProject(ownerID, projectID), update)
	if err != nil {
		return fmt.Errorf("rotate api key: %w", err)
	}
	if res.MatchedCount == 0 {
		return model.ErrNotFound
	}
	return nil
}

// DeleteProject removes a project and all its telemetry (logs, errors and
// metrics). It returns model.ErrNotFound if the owner has no such project.
//
// The project document goes first, so its API key stops working before the
// data is removed; a write already in the ingest queue may still add a few
// documents afterwards, which belong to no project and are never shown.
func (s *Store) DeleteProject(ctx context.Context, ownerID, projectID bson.ObjectID) error {
	res, err := s.Collection(CollApplications).DeleteOne(ctx, ownedProject(ownerID, projectID))
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if res.DeletedCount == 0 {
		return model.ErrNotFound
	}
	byProject := bson.D{{Key: "project_id", Value: projectID}}
	for _, coll := range []string{CollErrorLogs, CollAppLogs, CollTelemetryMetrics} {
		if _, err := s.Collection(coll).DeleteMany(ctx, byProject); err != nil {
			return fmt.Errorf("delete %s of project: %w", coll, err)
		}
	}
	return nil
}

// TouchProject records that data arrived with the project's key. It only
// writes when the stored time is missing or older than a minute, so busy
// projects cause at most one write per minute.
func (s *Store) TouchProject(ctx context.Context, projectID bson.ObjectID, now time.Time) error {
	filter := bson.D{
		{Key: "_id", Value: projectID},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "last_used_at", Value: bson.D{{Key: "$exists", Value: false}}}},
			bson.D{{Key: "last_used_at", Value: bson.D{{Key: "$lt", Value: now.Add(-time.Minute)}}}},
		}},
	}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "last_used_at", Value: now}}}}
	if _, err := s.Collection(CollApplications).UpdateOne(ctx, filter, update); err != nil {
		return fmt.Errorf("touch project: %w", err)
	}
	return nil
}
