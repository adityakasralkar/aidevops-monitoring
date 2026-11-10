package store

import (
	"context"
	"fmt"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// MaxCILogs caps how many CI reports one Pipelines request reads.
const MaxCILogs = 2_000

// attrCIRunID marks a log sent by a CI pipeline (one per run, or one per
// failed step). It contains dots, so it is read with $getField.
const attrCIRunID = "ci.run.id"

// CILogs returns the project's logs that came from a CI pipeline since
// `since`, newest first. They are ordinary logs (INFO for a passed run,
// ERROR / CRITICAL per failed step) carrying a "ci.run.id" attribute.
func (s *Store) CILogs(ctx context.Context, projectID bson.ObjectID, since time.Time) ([]model.LogRecord, error) {
	filter := bson.D{
		{Key: "project_id", Value: projectID},
		{Key: "timestamp", Value: bson.D{{Key: "$gte", Value: since}}},
		{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{
			bson.D{{Key: "$type", Value: bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: attrCIRunID}, {Key: "input", Value: "$attributes"},
			}}}}},
			"string",
		}}}},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(MaxCILogs)

	var all []model.LogRecord
	for _, coll := range []string{CollErrorLogs, CollAppLogs} {
		cur, err := s.Collection(coll).Find(ctx, filter, opts)
		if err != nil {
			return nil, fmt.Errorf("find ci logs in %s: %w", coll, err)
		}
		var page []model.LogRecord
		if err := cur.All(ctx, &page); err != nil {
			return nil, fmt.Errorf("read ci logs from %s: %w", coll, err)
		}
		all = append(all, page...)
	}
	slices.SortFunc(all, newestFirst)
	if len(all) > MaxCILogs {
		all = all[:MaxCILogs]
	}
	if all == nil {
		all = []model.LogRecord{}
	}
	return all, nil
}
