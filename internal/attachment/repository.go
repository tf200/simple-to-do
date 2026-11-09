package attachment

import (
	"context"
	"time"

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
	return &Repository{coll: db.Collection("attachments")}
}

// EnsureIndexes creates the indexes for listing a parent's files, the quota sum and the sweep.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "parent_kind", Value: 1}, {Key: "parent_id", Value: 1}, {Key: "created_at", Value: 1}}},
		{Keys: bson.D{{Key: "user_id", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: 1}}},
	})
	return err
}

func (r *Repository) Create(ctx context.Context, a *models.Attachment) error {
	_, err := r.coll.InsertOne(ctx, a)
	return err
}

// FindOwned returns the attachment when it belongs to the user, else mongo.ErrNoDocuments.
func (r *Repository) FindOwned(ctx context.Context, userID, id primitive.ObjectID) (*models.Attachment, error) {
	var a models.Attachment
	if err := r.coll.FindOne(ctx, bson.M{"_id": id, "user_id": userID}).Decode(&a); err != nil {
		return nil, err
	}
	return &a, nil
}

// FindByParent returns a parent's attachments with the given status, oldest first.
func (r *Repository) FindByParent(ctx context.Context, kind string, parentID primitive.ObjectID, status string) ([]models.Attachment, error) {
	filter := bson.M{"parent_kind": kind, "parent_id": parentID}
	if status != "" {
		filter["status"] = status
	}
	return r.find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
}

// FindByParents returns every attachment of the given parents, whatever its status.
func (r *Repository) FindByParents(ctx context.Context, kind string, parentIDs []primitive.ObjectID) ([]models.Attachment, error) {
	return r.find(ctx, bson.M{"parent_kind": kind, "parent_id": bson.M{"$in": parentIDs}}, nil)
}

// FindPendingBefore returns uploads started before `t` and never confirmed.
func (r *Repository) FindPendingBefore(ctx context.Context, t time.Time) ([]models.Attachment, error) {
	return r.find(ctx, bson.M{"status": StatusPending, "created_at": bson.M{"$lt": primitive.NewDateTimeFromTime(t)}}, nil)
}

func (r *Repository) find(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]models.Attachment, error) {
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	out := []models.Attachment{}
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CountByParent counts a parent's attachments, pending ones included.
func (r *Repository) CountByParent(ctx context.Context, kind string, parentID primitive.ObjectID) (int64, error) {
	return r.coll.CountDocuments(ctx, bson.M{"parent_kind": kind, "parent_id": parentID})
}

// UsedBytes is the total size of the user's attachments, pending ones included.
func (r *Repository) UsedBytes(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	cursor, err := r.coll.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"user_id": userID}}},
		{{Key: "$group", Value: bson.M{"_id": nil, "total": bson.M{"$sum": "$size"}}}},
	})
	if err != nil {
		return 0, err
	}
	var rows []struct {
		Total int64 `bson:"total"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].Total, nil
}

// MarkReady flips a pending attachment to ready; false if it wasn't pending any more.
func (r *Repository) MarkReady(ctx context.Context, id primitive.ObjectID) (bool, error) {
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": id, "status": StatusPending}, bson.M{"$set": bson.M{"status": StatusReady}})
	if err != nil {
		return false, err
	}
	return res.ModifiedCount == 1, nil
}

func (r *Repository) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}
