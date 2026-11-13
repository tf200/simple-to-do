package stage

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	models "tf200/todoapp/internal"
)

// Repository keeps one workflow document per user (its _id is the user's id).
type Repository struct {
	coll  *mongo.Collection
	lists *mongo.Collection
	tasks *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{coll: db.Collection("workflows"), lists: db.Collection("lists"), tasks: db.Collection("tasks")}
}

type workflow struct {
	UserID primitive.ObjectID `bson:"_id"`
	Stages []models.Stage     `bson:"stages"`
}

// Get returns the user's steps, or nil when they've never saved a workflow.
func (r *Repository) Get(ctx context.Context, userID primitive.ObjectID) ([]models.Stage, error) {
	var w workflow
	err := r.coll.FindOne(ctx, bson.M{"_id": userID}).Decode(&w)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return w.Stages, nil
}

func (r *Repository) Save(ctx context.Context, userID primitive.ObjectID, stages []models.Stage) error {
	_, err := r.coll.ReplaceOne(ctx, bson.M{"_id": userID}, workflow{UserID: userID, Stages: stages}, options.Replace().SetUpsert(true))
	return err
}

// MoveTasks moves the user's tasks in step `from` to step `to` ("" is to do).
func (r *Repository) MoveTasks(ctx context.Context, userID primitive.ObjectID, from, to string) error {
	cursor, err := r.lists.Find(ctx, bson.M{"user_id": userID}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return err
	}
	var rows []struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	ids := make([]primitive.ObjectID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	change := bson.M{"$unset": bson.M{"stage": ""}}
	if to != "" {
		change = bson.M{"$set": bson.M{"stage": to}}
	}
	_, err = r.tasks.UpdateMany(ctx, bson.M{"list_id": bson.M{"$in": ids}, "stage": from}, change)
	return err
}
