package auth

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Repository struct {
	coll      *mongo.Collection
	tokenColl *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		coll:      db.Collection("users"),
		tokenColl: db.Collection("api_tokens"),
	}
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := r.coll.FindOne(ctx, bson.M{"email": email}).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// EnsureIndexes makes emails unique, so two registrations can't create the same account.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return err
}

func (r *Repository) FindByID(ctx context.Context, id primitive.ObjectID) (*User, error) {
	var user User
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) Create(ctx context.Context, user *User) error {
	user.CreatedAt = time.Now()
	_, err := r.coll.InsertOne(ctx, user)
	return err
}

func (r *Repository) CreateAPIToken(ctx context.Context, token *APIToken) error {
	if token.ID.IsZero() {
		token.ID = primitive.NewObjectID()
	}
	token.CreatedAt = time.Now()
	_, err := r.tokenColl.InsertOne(ctx, token)
	return err
}

func (r *Repository) FindActiveAPITokenByHash(ctx context.Context, tokenHash string) (*APIToken, error) {
	var token APIToken
	filter := activeAPITokenFilter(bson.M{
		"token_hash": tokenHash,
	})
	err := r.tokenColl.FindOne(ctx, filter).Decode(&token)
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *Repository) FindActiveAPITokensByUser(ctx context.Context, userID primitive.ObjectID) ([]APIToken, error) {
	filter := activeAPITokenFilter(bson.M{
		"user_id": userID,
	})
	cursor, err := r.tokenColl.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var tokens []APIToken
	if err := cursor.All(ctx, &tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

func (r *Repository) RevokeAPIToken(ctx context.Context, userID, tokenID primitive.ObjectID) error {
	now := time.Now()
	filter := activeAPITokenFilter(bson.M{
		"_id":     tokenID,
		"user_id": userID,
	})
	result, err := r.tokenColl.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"revoked_at": now}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return errors.New("token not found")
	}
	return nil
}

func activeAPITokenFilter(filter bson.M) bson.M {
	filter["$or"] = []bson.M{
		{"revoked_at": bson.M{"$exists": false}},
		{"revoked_at": nil},
	}
	return filter
}
