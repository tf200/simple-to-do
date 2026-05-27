package list

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
	return &Repository{coll: db.Collection("lists")}
}

func (r *Repository) Create(ctx context.Context, list *models.List) error {
	list.ID = primitive.NewObjectID()
	_, err := r.coll.InsertOne(ctx, list)
	return err
}

func (r *Repository) FindByUser(ctx context.Context, userID primitive.ObjectID) ([]models.List, error) {
	cursor, err := r.coll.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	var lists []models.List
	if err := cursor.All(ctx, &lists); err != nil {
		return nil, err
	}
	return lists, nil
}

func (r *Repository) FindByID(ctx context.Context, id primitive.ObjectID) (*models.List, error) {
	var list models.List
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&list)
	if err != nil {
		return nil, err
	}
	return &list, nil
}

func (r *Repository) Update(ctx context.Context, id primitive.ObjectID, update bson.M) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": update})
	return err
}

func (r *Repository) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}
