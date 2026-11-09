package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type User struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Email     string             `bson:"email" json:"email"`
	Password  string             `bson:"password" json:"-"`
	CreatedAt primitive.DateTime `bson:"created_at" json:"created_at"`
}

type List struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	Title     string             `bson:"title" json:"title"`
	CreatedAt primitive.DateTime `bson:"created_at" json:"created_at"`
}

type Task struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	ListID      primitive.ObjectID  `bson:"list_id" json:"list_id"`
	Title       string              `bson:"title" json:"title"`
	Description string              `bson:"description" json:"description"`
	DueAt       *primitive.DateTime `bson:"due_at,omitempty" json:"due_at,omitempty"`
	Done        bool                `bson:"done" json:"done"`
	CreatedAt   primitive.DateTime  `bson:"created_at" json:"created_at"`
}

type Note struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	Title     string             `bson:"title" json:"title"`
	Body      string             `bson:"body" json:"body"`
	Pinned    bool               `bson:"pinned" json:"pinned"`
	CreatedAt primitive.DateTime `bson:"created_at" json:"created_at"`
	UpdatedAt primitive.DateTime `bson:"updated_at" json:"updated_at"`
}
