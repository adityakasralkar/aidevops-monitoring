//go:build integration

package store

import (
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// newSchemaStore returns a test store with all collections and indexes created.
func newSchemaStore(t *testing.T) *Store {
	t.Helper()
	s := newTestStore(t)
	if err := s.EnsureSchema(testCtx(t)); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	return s
}

func TestUsers(t *testing.T) {
	s := newSchemaStore(t)
	ctx := testCtx(t)
	now := time.Now().UTC().Truncate(time.Millisecond)

	u := model.User{Email: "user@example.com", PasswordHash: "hash", CreatedAt: now}
	if err := s.CreateUser(ctx, &u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if u.ID.IsZero() {
		t.Fatal("CreateUser did not set the id")
	}

	t.Run("rejects a duplicate email", func(t *testing.T) {
		dup := model.User{Email: "user@example.com", PasswordHash: "other", CreatedAt: now}
		if err := s.CreateUser(ctx, &dup); !errors.Is(err, model.ErrEmailTaken) {
			t.Errorf("err = %v, want ErrEmailTaken", err)
		}
	})
	t.Run("finds by email", func(t *testing.T) {
		got, err := s.UserByEmail(ctx, "user@example.com")
		if err != nil || got != u {
			t.Errorf("got %+v, %v; want %+v", got, err, u)
		}
	})
	t.Run("finds by id", func(t *testing.T) {
		got, err := s.UserByID(ctx, u.ID)
		if err != nil || got != u {
			t.Errorf("got %+v, %v; want %+v", got, err, u)
		}
	})
	t.Run("returns ErrNotFound for an unknown email", func(t *testing.T) {
		if _, err := s.UserByEmail(ctx, "nobody@example.com"); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("returns ErrNotFound for an unknown id", func(t *testing.T) {
		if _, err := s.UserByID(ctx, bson.NewObjectID()); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
}

func TestSessions(t *testing.T) {
	s := newSchemaStore(t)
	ctx := testCtx(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	userID := bson.NewObjectID()

	active := model.Session{UserID: userID, TokenHash: "active", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	expired := model.Session{UserID: userID, TokenHash: "expired", CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)}
	for _, sess := range []*model.Session{&active, &expired} {
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}

	t.Run("finds an active session", func(t *testing.T) {
		got, err := s.SessionByTokenHash(ctx, "active", now)
		if err != nil || got != active {
			t.Errorf("got %+v, %v; want %+v", got, err, active)
		}
	})
	t.Run("hides an expired session", func(t *testing.T) {
		if _, err := s.SessionByTokenHash(ctx, "expired", now); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("rejects a duplicate token hash", func(t *testing.T) {
		dup := model.Session{UserID: userID, TokenHash: "active", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
		if err := s.CreateSession(ctx, &dup); err == nil {
			t.Error("expected an error, got nil")
		}
	})
	t.Run("deletes a session", func(t *testing.T) {
		if err := s.DeleteSession(ctx, "active"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := s.SessionByTokenHash(ctx, "active", now); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound after delete", err)
		}
	})
	t.Run("deleting a missing session is not an error", func(t *testing.T) {
		if err := s.DeleteSession(ctx, "never-existed"); err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})
}

func TestProjects(t *testing.T) {
	s := newSchemaStore(t)
	ctx := testCtx(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	alice, bob := bson.NewObjectID(), bson.NewObjectID()

	newProject := func(owner bson.ObjectID, name, env, hash string, created time.Time) *model.Project {
		return &model.Project{OwnerID: owner, Name: name, Environment: env, APIKeyHash: hash, KeyPrefix: "aidv_" + env + "_abcd", CreatedAt: created}
	}

	older := newProject(alice, "shop", "test", "h1", now.Add(-time.Minute))
	newer := newProject(alice, "shop", "live", "h2", now)
	other := newProject(bob, "shop", "test", "h3", now)
	for _, p := range []*model.Project{older, newer, other} {
		if err := s.CreateProject(ctx, p); err != nil {
			t.Fatalf("create project: %v", err)
		}
		if p.ID.IsZero() {
			t.Fatal("CreateProject did not set the id")
		}
	}

	t.Run("rejects the same name and environment for the same owner", func(t *testing.T) {
		dup := newProject(alice, "shop", "test", "h4", now)
		if err := s.CreateProject(ctx, dup); !errors.Is(err, model.ErrProjectExists) {
			t.Errorf("err = %v, want ErrProjectExists", err)
		}
	})
	t.Run("rejects a duplicate API key hash", func(t *testing.T) {
		dup := newProject(alice, "other", "test", "h1", now)
		err := s.CreateProject(ctx, dup)
		if err == nil || errors.Is(err, model.ErrProjectExists) {
			t.Errorf("err = %v, want a non-ErrProjectExists error", err)
		}
	})
	t.Run("lists only the owner's projects, newest first, without key hashes", func(t *testing.T) {
		got, err := s.ProjectsByOwner(ctx, alice)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 2 || got[0].ID != newer.ID || got[1].ID != older.ID {
			t.Fatalf("got %+v; want [newer, older]", got)
		}
		for _, p := range got {
			if p.APIKeyHash != "" {
				t.Errorf("project %s includes the key hash", p.ID.Hex())
			}
			if p.OwnerID != alice {
				t.Errorf("project %s belongs to someone else", p.ID.Hex())
			}
		}
	})
	t.Run("returns an empty list, not nil, for a user with no projects", func(t *testing.T) {
		got, err := s.ProjectsByOwner(ctx, bson.NewObjectID())
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("got %#v, %v; want empty non-nil slice", got, err)
		}
	})
}
