package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// CreateUser inserts a new user and sets u.ID.
// It returns model.ErrEmailTaken if the email is already registered.
func (s *Store) CreateUser(ctx context.Context, u *model.User) error {
	res, err := s.Collection(CollUsers).InsertOne(ctx, u)
	if isDuplicateOn(err, indexUserEmail) {
		return model.ErrEmailTaken
	}
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	u.ID = res.InsertedID.(bson.ObjectID)
	return nil
}

// UserByEmail finds a user by (already normalised) email.
// It returns model.ErrNotFound if there is no such user.
func (s *Store) UserByEmail(ctx context.Context, email string) (model.User, error) {
	return s.findUser(ctx, bson.D{{Key: "email", Value: email}})
}

// UserByID finds a user by id. It returns model.ErrNotFound if there is no such user.
func (s *Store) UserByID(ctx context.Context, id bson.ObjectID) (model.User, error) {
	return s.findUser(ctx, bson.D{{Key: "_id", Value: id}})
}

func (s *Store) findUser(ctx context.Context, filter bson.D) (model.User, error) {
	var u model.User
	err := s.Collection(CollUsers).FindOne(ctx, filter).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.User{}, model.ErrNotFound
	}
	if err != nil {
		return model.User{}, fmt.Errorf("find user: %w", err)
	}
	return u, nil
}

// isDuplicateOn reports whether err is a duplicate-key error on the named index.
// MongoDB names the index in the error message ("... index: email_unique dup key ...").
func isDuplicateOn(err error, indexName string) bool {
	return err != nil && mongo.IsDuplicateKeyError(err) && strings.Contains(err.Error(), indexName)
}
