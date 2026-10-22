// Package store handles the MongoDB connection, collections and indexes.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// Collection names.
const (
	CollUsers            = "Users"
	CollSessions         = "Sessions"
	CollApplications     = "Applications"
	CollErrorLogs        = "ErrorLogs"
	CollAppLogs          = "AppLogs"
	CollTelemetryMetrics = "TelemetryMetrics"
)

// serverSelectionTimeout is how long the driver waits to find a reachable
// MongoDB server before giving up. The driver default is 30s; 5s means an
// outage shows up as an error quickly instead of making requests hang.
const serverSelectionTimeout = 5 * time.Second

// codeNamespaceExists is MongoDB's error code for "collection already exists".
const codeNamespaceExists = 48

// Names of unique indexes whose duplicate-key errors are turned into
// friendly errors (see users.go and projects.go).
const (
	indexUserEmail       = "email_unique"
	indexOwnerProjectEnv = "owner_project_env_unique"
)

// schema lists every collection and the indexes it needs.
// Indexes have fixed names so they are easy to recognise in Compass.
var schema = []struct {
	name    string
	indexes []mongo.IndexModel
}{
	// One account per email address.
	{CollUsers, []mongo.IndexModel{
		index(indexUserEmail, bson.D{{Key: "email", Value: 1}}).unique(),
	}},
	// Token lookup on every logged-in request; MongoDB deletes sessions
	// automatically once expires_at has passed; user_id finds a user's sessions.
	{CollSessions, []mongo.IndexModel{
		index("token_hash_unique", bson.D{{Key: "token_hash", Value: 1}}).unique(),
		index("expires_at_ttl", bson.D{{Key: "expires_at", Value: 1}}).ttl(),
		index("user_id", bson.D{{Key: "user_id", Value: 1}}).model(),
	}},
	// Fast API-key lookup on every request; unique so no two projects share a key.
	// A user cannot have two projects with the same name in the same environment.
	// owner_created lists a user's projects newest first.
	{CollApplications, []mongo.IndexModel{
		index("api_key_hash_unique", bson.D{{Key: "api_key_hash", Value: 1}}).unique(),
		index(indexOwnerProjectEnv, bson.D{{Key: "owner_id", Value: 1}, {Key: "project_name", Value: 1}, {Key: "environment", Value: 1}}).unique(),
		index("owner_created", bson.D{{Key: "owner_id", Value: 1}, {Key: "created_at", Value: -1}}).model(),
	}},
	// Latest errors per project, and errors still waiting for diagnosis.
	{CollErrorLogs, []mongo.IndexModel{
		index("project_timestamp", bson.D{{Key: "project_id", Value: 1}, {Key: "timestamp", Value: -1}}).model(),
		index("project_diagnosis_status", bson.D{{Key: "project_id", Value: 1}, {Key: "diagnosis_status", Value: 1}}).model(),
	}},
	// Latest logs per project.
	{CollAppLogs, []mongo.IndexModel{
		index("project_timestamp", bson.D{{Key: "project_id", Value: 1}, {Key: "timestamp", Value: -1}}).model(),
	}},
	// Latest metrics per project, and one metric over time (for charts).
	{CollTelemetryMetrics, []mongo.IndexModel{
		index("project_timestamp", bson.D{{Key: "project_id", Value: 1}, {Key: "timestamp", Value: -1}}).model(),
		index("project_metric_timestamp", bson.D{{Key: "project_id", Value: 1}, {Key: "metric_name", Value: 1}, {Key: "timestamp", Value: -1}}).model(),
	}},
}

// Store holds the MongoDB client and the application's database.
type Store struct {
	client *mongo.Client
	db     *mongo.Database
}

// Connect connects to MongoDB and pings it to make sure it is reachable.
// It returns an error if MongoDB cannot be reached before ctx runs out.
func Connect(ctx context.Context, uri, dbName string) (*Store, error) {
	opts := options.Client().
		ApplyURI(uri).
		SetAppName("aidevops-server"). // shows up in MongoDB's own logs
		SetServerSelectionTimeout(serverSelectionTimeout)

	// This only sets up the client; it does not contact the server yet.
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("create mongo client: %w", err)
	}

	// Ping now, so a wrong URI or a stopped database is caught at startup.
	s := &Store{client: client, db: client.Database(dbName)}
	if err := s.Ping(ctx); err != nil {
		// Close the client so its background connections do not leak.
		_ = client.Disconnect(context.WithoutCancel(ctx))
		return nil, err
	}
	return s, nil
}

// Ping checks that the primary server (the one that accepts writes) is reachable.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("ping mongo: %w", err)
	}
	return nil
}

// EnsureSchema creates any missing collections and indexes.
//
// It is safe to run on every start: things that already exist are left
// alone. If an index with the same name but a different definition exists,
// it returns an error instead of silently keeping the wrong index.
func (s *Store) EnsureSchema(ctx context.Context) error {
	for _, c := range schema {
		// "Already exists" is fine; any other error is a real problem.
		if err := s.db.CreateCollection(ctx, c.name); err != nil && !isNamespaceExists(err) {
			return fmt.Errorf("create collection %s: %w", c.name, err)
		}
		// Creating an index that already exists with the same definition does nothing.
		if _, err := s.db.Collection(c.name).Indexes().CreateMany(ctx, c.indexes); err != nil {
			return fmt.Errorf("create indexes on %s: %w", c.name, err)
		}
	}
	return nil
}

// Collection returns the collection with the given name.
func (s *Store) Collection(name string) *mongo.Collection {
	return s.db.Collection(name)
}

// Close disconnects from MongoDB. It waits for running operations to
// finish, but no longer than ctx allows.
func (s *Store) Close(ctx context.Context) error {
	if err := s.client.Disconnect(ctx); err != nil {
		return fmt.Errorf("disconnect mongo: %w", err)
	}
	return nil
}

// isNamespaceExists reports whether err means "collection already exists".
func isNamespaceExists(err error) bool {
	var se mongo.ServerError
	return errors.As(err, &se) && se.HasErrorCode(codeNamespaceExists)
}

// indexSpec is a small helper that keeps the schema list above short.
// Usage: index("name", keys).model() or index("name", keys).unique().
type indexSpec struct {
	name string
	keys bson.D
}

func index(name string, keys bson.D) indexSpec {
	return indexSpec{name: name, keys: keys}
}

// model returns a normal index.
func (i indexSpec) model() mongo.IndexModel {
	return mongo.IndexModel{Keys: i.keys, Options: options.Index().SetName(i.name)}
}

// unique returns an index that rejects duplicate values.
func (i indexSpec) unique() mongo.IndexModel {
	return mongo.IndexModel{Keys: i.keys, Options: options.Index().SetName(i.name).SetUnique(true)}
}

// ttl returns an index that makes MongoDB delete each document once the
// indexed date has passed. MongoDB checks about once a minute.
func (i indexSpec) ttl() mongo.IndexModel {
	return mongo.IndexModel{Keys: i.keys, Options: options.Index().SetName(i.name).SetExpireAfterSeconds(0)}
}
