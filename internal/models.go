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
	// The workflow step the task is in (a Stage id); empty for to do and done.
	Stage     string             `bson:"stage,omitempty" json:"stage,omitempty"`
	CreatedAt primitive.DateTime `bson:"created_at" json:"created_at"`
}

// Stage is a step of a user's workflow, between to do and done.
type Stage struct {
	ID    string `bson:"id" json:"id"`
	Name  string `bson:"name" json:"name"`
	Color string `bson:"color" json:"color"`
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

// Attachment is a file on a task or a note. The bytes live in the S3 store under Key; this is
// what the API knows about them. Pending ones have an upload link out but no confirmed upload.
type Attachment struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID     primitive.ObjectID `bson:"user_id" json:"-"`
	ParentKind string             `bson:"parent_kind" json:"parent_kind"`
	ParentID   primitive.ObjectID `bson:"parent_id" json:"parent_id"`
	Name       string             `bson:"name" json:"name"`
	Mime       string             `bson:"mime" json:"mime"`
	Size       int64              `bson:"size" json:"size"`
	Key        string             `bson:"key" json:"-"`
	Status     string             `bson:"status" json:"status"`
	CreatedAt  primitive.DateTime `bson:"created_at" json:"created_at"`
}
