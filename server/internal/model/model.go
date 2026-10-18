// Package model defines the data shapes shared by the other packages.
// It imports no other internal package.
package model

import (
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Errors returned by the store and understood by the API layer.
var (
	ErrNotFound      = errors.New("not found")
	ErrEmailTaken    = errors.New("email already registered")
	ErrProjectExists = errors.New("project already exists")
)

// User is an account that can log in and own projects.
type User struct {
	ID           bson.ObjectID `bson:"_id,omitempty"`
	Email        string        `bson:"email"`
	PasswordHash string        `bson:"password_hash"`
	CreatedAt    time.Time     `bson:"created_at"`
}

// Session is one logged-in device or client. The token itself is never
// stored, only its hash.
type Session struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	UserID    bson.ObjectID `bson:"user_id"`
	TokenHash string        `bson:"token_hash"`
	CreatedAt time.Time     `bson:"created_at"`
	ExpiresAt time.Time     `bson:"expires_at"`
}

// Project is a registered application that sends telemetry with its API key.
// It is stored in the Applications collection.
type Project struct {
	ID          bson.ObjectID `bson:"_id,omitempty"`
	OwnerID     bson.ObjectID `bson:"owner_id"`
	Name        string        `bson:"project_name"`
	Environment string        `bson:"environment"`
	APIKeyHash  string        `bson:"api_key_hash"`
	KeyPrefix   string        `bson:"key_prefix"`
	CreatedAt   time.Time     `bson:"created_at"`
}
