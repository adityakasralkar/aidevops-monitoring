package store

import (
	"context"
	"errors"
	"fmt"

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
