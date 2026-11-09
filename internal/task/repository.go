package task

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	models "tf200/todoapp/internal"
)

type Repository struct {
	coll *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{coll: db.Collection("tasks")}
}

func (r *Repository) Create(ctx context.Context, task *models.Task) error {
	task.ID = primitive.NewObjectID()
	_, err := r.coll.InsertOne(ctx, task)
	return err
}

func (r *Repository) FindByList(ctx context.Context, listID primitive.ObjectID) ([]models.Task, error) {
	cursor, err := r.coll.Find(ctx, bson.M{"list_id": listID})
	if err != nil {
		return nil, err
	}
	var tasks []models.Task
	if err := cursor.All(ctx, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *Repository) FindByID(ctx context.Context, id primitive.ObjectID) (*models.Task, error) {
	var task models.Task
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&task)
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// Update sets the fields in `set` and removes those in `unset`; either may be empty.
func (r *Repository) Update(ctx context.Context, id primitive.ObjectID, set, unset bson.M) error {
	change := bson.M{}
	if len(set) > 0 {
		change["$set"] = set
	}
	if len(unset) > 0 {
		change["$unset"] = unset
	}
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, change)
	return err
}

func (r *Repository) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}
