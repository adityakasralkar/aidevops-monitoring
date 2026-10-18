//go:build integration

package store

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// newTestStore connects to MONGO_URI (default: local Docker) using a
// throwaway database that is dropped when the test ends.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}
	dbName := fmt.Sprintf("aidevops_test_%d", time.Now().UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := Connect(ctx, uri, dbName)
	if err != nil {
		t.Fatalf("connect: %v (is MongoDB running? try: make up)", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.db.Drop(ctx)
		_ = s.Close(ctx)
	})
	return s
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func indexNames(t *testing.T, ctx context.Context, coll *mongo.Collection) []string {
	t.Helper()
	specs, err := coll.Indexes().ListSpecifications(ctx)
	if err != nil {
		t.Fatalf("list indexes on %s: %v", coll.Name(), err)
	}
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.Name)
	}
	return names
}

func TestPingSucceeds(t *testing.T) {
	s := newTestStore(t)
	if err := s.Ping(testCtx(t)); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestEnsureSchemaCreatesCollectionsAndIndexes(t *testing.T) {
	s := newTestStore(t)
	ctx := testCtx(t)

	if err := s.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	collections, err := s.db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		t.Fatalf("list collections: %v", err)
	}

	want := map[string][]string{
		CollUsers:            {"email_unique"},
		CollSessions:         {"token_hash_unique", "expires_at_ttl", "user_id"},
		CollApplications:     {"api_key_hash_unique", "owner_project_env_unique", "owner_created"},
		CollErrorLogs:        {"project_timestamp", "project_diagnosis_status"},
		CollAppLogs:          {"project_timestamp"},
		CollTelemetryMetrics: {"project_timestamp"},
	}
	for coll, wantIndexes := range want {
		t.Run(coll, func(t *testing.T) {
			if !slices.Contains(collections, coll) {
				t.Fatalf("collection %s not created; have %v", coll, collections)
			}
			got := indexNames(t, ctx, s.Collection(coll))
			for _, name := range wantIndexes {
				if !slices.Contains(got, name) {
					t.Errorf("index %s missing; have %v", name, got)
				}
			}
		})
	}
}

func TestEnsureSchemaIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := testCtx(t)

	for i := range 3 {
		if err := s.EnsureSchema(ctx); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
}

func TestApiKeyHashIsUnique(t *testing.T) {
	s := newTestStore(t)
	ctx := testCtx(t)
	if err := s.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	apps := s.Collection(CollApplications)
	doc := bson.D{{Key: "project_name", Value: "demo"}, {Key: "api_key_hash", Value: "abc123"}}
	if _, err := apps.InsertOne(ctx, doc); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	_, err := apps.InsertOne(ctx, doc)
	if !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("second insert: want duplicate key error, got %v", err)
	}
}

func TestEnsureSchemaRejectsConflictingIndex(t *testing.T) {
	s := newTestStore(t)
	ctx := testCtx(t)

	// Same name as a schema index but different keys: must not be silently replaced.
	_, err := s.Collection(CollAppLogs).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "message", Value: 1}},
		Options: options.Index().SetName("project_timestamp"),
	})
	if err != nil {
		t.Fatalf("create conflicting index: %v", err)
	}

	if err := s.EnsureSchema(ctx); err == nil {
		t.Fatal("expected an error for the conflicting index, got nil")
	}
}
