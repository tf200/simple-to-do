package note

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	models "tf200/todoapp/internal"
)

type Repository struct {
	coll *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{coll: db.Collection("notes")}
}

// EnsureIndexes creates the index the notes list query uses.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "updated_at", Value: -1}},
	})
	return err
}

func (r *Repository) Create(ctx context.Context, note *models.Note) error {
	note.ID = primitive.NewObjectID()
	_, err := r.coll.InsertOne(ctx, note)
	return err
}

// FindByUser returns the user's notes, most recently edited first.
func (r *Repository) FindByUser(ctx context.Context, userID primitive.ObjectID) ([]models.Note, error) {
	opts := options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}})
	cursor, err := r.coll.Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	notes := []models.Note{}
	if err := cursor.All(ctx, &notes); err != nil {
		return nil, err
	}
	return notes, nil
}

// FindOwned returns the note when it belongs to the user, else mongo.ErrNoDocuments.
func (r *Repository) FindOwned(ctx context.Context, userID, id primitive.ObjectID) (*models.Note, error) {
	var note models.Note
	err := r.coll.FindOne(ctx, bson.M{"_id": id, "user_id": userID}).Decode(&note)
	if err != nil {
		return nil, err
	}
	return &note, nil
}

func (r *Repository) Update(ctx context.Context, userID, id primitive.ObjectID, update bson.M) error {
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": id, "user_id": userID}, bson.M{"$set": update})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, userID, id primitive.ObjectID) error {
	res, err := r.coll.DeleteOne(ctx, bson.M{"_id": id, "user_id": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}
