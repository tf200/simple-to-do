package auth

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

type CreateAPITokenRequest struct {
	Name string `json:"name" binding:"required"`
}

type CreateAPITokenResponse struct {
	ID        primitive.ObjectID `json:"id"`
	Name      string             `json:"name"`
	Token     string             `json:"token"`
	CreatedAt time.Time          `json:"created_at"`
}

type APITokenResponse struct {
	ID        primitive.ObjectID `json:"id"`
	Name      string             `json:"name"`
	CreatedAt time.Time          `json:"created_at"`
}

type UserResponse struct {
	ID        primitive.ObjectID `bson:"_id" json:"id"`
	Email     string             `bson:"email" json:"email"`
	CreatedAt primitive.DateTime `bson:"created_at" json:"created_at"`
}

type User struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	Email     string             `bson:"email"`
	Password  string             `bson:"password"`
	CreatedAt time.Time          `bson:"created_at"`
}

type APIToken struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	Name      string             `bson:"name" json:"name"`
	TokenHash string             `bson:"token_hash" json:"-"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	RevokedAt *time.Time         `bson:"revoked_at,omitempty" json:"revoked_at,omitempty"`
}
