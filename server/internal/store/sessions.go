package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// CreateSession inserts a new session and sets sess.ID.
func (s *Store) CreateSession(ctx context.Context, sess *model.Session) error {
	res, err := s.Collection(CollSessions).InsertOne(ctx, sess)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	sess.ID = res.InsertedID.(bson.ObjectID)
	return nil
}

// SessionByTokenHash finds a session that has not expired at time now.
// It returns model.ErrNotFound if the session is missing or expired.
//
// Expiry is checked here as well because MongoDB's TTL cleanup only runs
// about once a minute, so an expired session may still exist briefly.
func (s *Store) SessionByTokenHash(ctx context.Context, tokenHash string, now time.Time) (model.Session, error) {
	filter := bson.D{
		{Key: "token_hash", Value: tokenHash},
		{Key: "expires_at", Value: bson.D{{Key: "$gt", Value: now}}},
	}
	var sess model.Session
	err := s.Collection(CollSessions).FindOne(ctx, filter).Decode(&sess)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.Session{}, model.ErrNotFound
	}
	if err != nil {
		return model.Session{}, fmt.Errorf("find session: %w", err)
	}
	return sess, nil
}

// DeleteSession removes the session with this token hash.
// Deleting a session that does not exist is not an error.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.Collection(CollSessions).DeleteOne(ctx, bson.D{{Key: "token_hash", Value: tokenHash}})
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
