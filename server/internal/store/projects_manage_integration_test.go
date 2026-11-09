//go:build integration

package store

import (
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

func TestProjectManagement(t *testing.T) {
	s := newSchemaStore(t)
	ctx := testCtx(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	owner, stranger := bson.NewObjectID(), bson.NewObjectID()

	mk := func(name, env, hash string) *model.Project {
		p := &model.Project{OwnerID: owner, Name: name, Environment: env, APIKeyHash: hash, KeyPrefix: "aidv_" + env + "_abcd", CreatedAt: now}
		if err := s.CreateProject(ctx, p); err != nil {
			t.Fatalf("create: %v", err)
		}
		return p
	}
	shop := mk("shop", "live", "h-shop")
	mk("billing", "live", "h-billing")

	t.Run("ProjectByID hides the key hash and other owners", func(t *testing.T) {
		got, err := s.ProjectByID(ctx, owner, shop.ID)
		if err != nil || got.Name != "shop" || got.APIKeyHash != "" {
			t.Fatalf("got %+v, %v", got, err)
		}
		if _, err := s.ProjectByID(ctx, stranger, shop.ID); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("stranger: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("RenameProject", func(t *testing.T) {
		if err := s.RenameProject(ctx, owner, shop.ID, "billing"); !errors.Is(err, model.ErrProjectExists) {
			t.Errorf("duplicate: err = %v", err)
		}
		if err := s.RenameProject(ctx, stranger, shop.ID, "x"); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("stranger: err = %v", err)
		}
		if err := s.RenameProject(ctx, owner, shop.ID, "shop-backend"); err != nil {
			t.Fatalf("rename: %v", err)
		}
		got, _ := s.ProjectByID(ctx, owner, shop.ID)
		if got.Name != "shop-backend" {
			t.Errorf("name = %q", got.Name)
		}
	})

	t.Run("TouchProject writes at most once a minute", func(t *testing.T) {
		if err := s.TouchProject(ctx, shop.ID, now); err != nil {
			t.Fatal(err)
		}
		if err := s.TouchProject(ctx, shop.ID, now.Add(30*time.Second)); err != nil {
			t.Fatal(err)
		}
		got, _ := s.ProjectByID(ctx, owner, shop.ID)
		if got.LastUsedAt == nil || !got.LastUsedAt.Equal(now) {
			t.Fatalf("last_used_at = %v, want %s (second touch ignored)", got.LastUsedAt, now)
		}
		later := now.Add(2 * time.Minute)
		_ = s.TouchProject(ctx, shop.ID, later)
		got, _ = s.ProjectByID(ctx, owner, shop.ID)
		if !got.LastUsedAt.Equal(later) {
			t.Errorf("last_used_at = %v, want %s", got.LastUsedAt, later)
		}
	})

	t.Run("RotateAPIKey replaces the key and clears last used", func(t *testing.T) {
		rotated := now.Add(time.Hour)
		if err := s.RotateAPIKey(ctx, owner, shop.ID, "h-new", "aidv_live_wxyz", rotated); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ProjectByAPIKeyHash(ctx, "h-shop"); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("old key still works: %v", err)
		}
		got, err := s.ProjectByAPIKeyHash(ctx, "h-new")
		if err != nil || got.ID != shop.ID || got.KeyPrefix != "aidv_live_wxyz" || got.LastUsedAt != nil || !got.KeyCreatedAt.Equal(rotated) {
			t.Errorf("got %+v, %v", got, err)
		}
		if err := s.RotateAPIKey(ctx, stranger, shop.ID, "h-x", "p", rotated); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("stranger: err = %v", err)
		}
	})

	t.Run("DeleteProject removes the project and only its data", func(t *testing.T) {
		otherProject := bson.NewObjectID()
		_ = s.InsertErrorLogs(ctx, append(testLogs(2, model.SeverityError, shop.ID), testLogs(1, model.SeverityError, otherProject)...))
		_ = s.InsertAppLogs(ctx, testLogs(3, model.SeverityInfo, shop.ID))
		v := 1.0
		_ = s.InsertMetrics(ctx, []model.MetricPoint{{ID: bson.NewObjectID(), ProjectID: shop.ID, MetricName: "m", MetricType: "gauge", Value: &v, Timestamp: now, ReceivedAt: now}})

		if err := s.DeleteProject(ctx, stranger, shop.ID); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("stranger: err = %v", err)
		}
		if err := s.DeleteProject(ctx, owner, shop.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ProjectByID(ctx, owner, shop.ID); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("project still exists: %v", err)
		}
		for _, coll := range []string{CollErrorLogs, CollAppLogs, CollTelemetryMetrics} {
			if n, _ := s.Collection(coll).CountDocuments(ctx, bson.D{{Key: "project_id", Value: shop.ID}}); n != 0 {
				t.Errorf("%s still has %d documents", coll, n)
			}
		}
		if n, _ := s.Collection(CollErrorLogs).CountDocuments(ctx, bson.D{{Key: "project_id", Value: otherProject}}); n != 1 {
			t.Errorf("another project's errors were deleted: %d left", n)
		}
	})
}
