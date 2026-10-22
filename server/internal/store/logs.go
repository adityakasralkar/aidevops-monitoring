package store

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// codeDuplicateKey is MongoDB's error code for a duplicate unique value.
const codeDuplicateKey = 11000

// InsertErrorLogs saves ERROR and CRITICAL logs.
func (s *Store) InsertErrorLogs(ctx context.Context, logs []model.LogRecord) error {
	return s.insertLogs(ctx, CollErrorLogs, logs)
}

// InsertAppLogs saves INFO and WARN logs.
func (s *Store) InsertAppLogs(ctx context.Context, logs []model.LogRecord) error {
	return s.insertLogs(ctx, CollAppLogs, logs)
}

// insertLogs saves a batch in one round trip.
//
// The batch is unordered, so one bad document does not stop the others.
// Each log already has its _id, so if an earlier attempt saved part of the
// batch, retrying it only produces "duplicate key" errors for the logs
// that are already stored. Those are treated as success: the data is there.
func (s *Store) insertLogs(ctx context.Context, coll string, logs []model.LogRecord) error {
	if len(logs) == 0 {
		return nil
	}
	_, err := s.Collection(coll).InsertMany(ctx, logs, options.InsertMany().SetOrdered(false))
	if err == nil || onlyDuplicates(err) {
		return nil
	}
	return fmt.Errorf("insert %d logs into %s: %w", len(logs), coll, err)
}

// onlyDuplicates reports whether every failure in a batch insert was a
// duplicate _id, meaning all documents are in fact stored.
func onlyDuplicates(err error) bool {
	var bwe mongo.BulkWriteException
	if !errors.As(err, &bwe) || bwe.WriteConcernError != nil || len(bwe.WriteErrors) == 0 {
		return false
	}
	for _, we := range bwe.WriteErrors {
		if we.Code != codeDuplicateKey {
			return false
		}
	}
	return true
}
