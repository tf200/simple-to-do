package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Sessions: a short-lived access JWT plus a long-lived refresh token.
//
// A refresh token works once. Using it returns a new pair and marks it used; the new refresh
// token belongs to the same family (one family per sign-in). Presenting a used token again means
// it was copied, so the whole family is revoked and that sign-in ends everywhere. The exception is
// a short grace window, so a client whose refresh answer was lost on the network can retry.
const (
	AccessTTL   = 15 * time.Minute
	RefreshTTL  = 60 * 24 * time.Hour
	reuseGrace  = time.Minute
	refreshKind = "rt_"
)

var ErrInvalidRefresh = errors.New("invalid refresh token")

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type RefreshToken struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	UserID    primitive.ObjectID `bson:"user_id"`
	FamilyID  primitive.ObjectID `bson:"family_id"`
	TokenHash string             `bson:"token_hash"`
	CreatedAt time.Time          `bson:"created_at"`
	ExpiresAt time.Time          `bson:"expires_at"`
	UsedAt    *time.Time         `bson:"used_at,omitempty"`
}

// Refresh swaps a refresh token for a new access token and refresh token.
func (s *Service) Refresh(ctx context.Context, raw string) (*LoginResponse, error) {
	now := time.Now()
	hash := HashAPIToken(raw)
	tok, err := s.repo.ClaimRefreshToken(ctx, hash, now)
	if errors.Is(err, mongo.ErrNoDocuments) {
		tok, err = s.replayed(ctx, hash, now)
	}
	if err != nil {
		return nil, err
	}
	user, err := s.repo.FindByID(ctx, tok.UserID)
	if err != nil {
		// The account is gone: so is the session.
		_ = s.repo.RevokeRefreshFamily(ctx, tok.FamilyID)
		return nil, ErrInvalidRefresh
	}
	return s.issueSession(ctx, user, tok.FamilyID)
}

// replayed handles a token that couldn't be claimed: unknown, expired, or already used. Used
// moments ago is most likely the same client retrying after losing the answer, so it may go on;
// used earlier means the token was copied, and the whole sign-in is revoked.
func (s *Service) replayed(ctx context.Context, hash string, now time.Time) (*RefreshToken, error) {
	tok, err := s.repo.FindRefreshToken(ctx, hash)
	if err != nil || !tok.ExpiresAt.After(now) || tok.UsedAt == nil {
		return nil, ErrInvalidRefresh
	}
	if now.Sub(*tok.UsedAt) <= reuseGrace {
		return tok, nil
	}
	_ = s.repo.RevokeRefreshFamily(ctx, tok.FamilyID)
	return nil, ErrInvalidRefresh
}

// Logout ends the sign-in this refresh token belongs to. Unknown tokens are ignored.
func (s *Service) Logout(ctx context.Context, raw string) error {
	tok, err := s.repo.FindRefreshToken(ctx, HashAPIToken(raw))
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.repo.RevokeRefreshFamily(ctx, tok.FamilyID)
}

// issueSession signs an access token and stores a new refresh token in `family`.
func (s *Service) issueSession(ctx context.Context, user *User, family primitive.ObjectID) (*LoginResponse, error) {
	now := time.Now()
	expires := now.Add(AccessTTL)
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   user.ID.Hex(),
		"email": user.Email,
		"iat":   now.Unix(),
		"exp":   expires.Unix(),
	}).SignedString(s.jwtSecret)
	if err != nil {
		return nil, err
	}

	raw, err := newRefreshToken()
	if err != nil {
		return nil, err
	}
	err = s.repo.CreateRefreshToken(ctx, &RefreshToken{
		ID:        primitive.NewObjectID(),
		UserID:    user.ID,
		FamilyID:  family,
		TokenHash: HashAPIToken(raw),
		CreatedAt: now,
		ExpiresAt: now.Add(RefreshTTL),
	})
	if err != nil {
		return nil, err
	}
	return &LoginResponse{Token: access, RefreshToken: raw, ExpiresAt: expires.Unix()}, nil
}

func newRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return refreshKind + base64.RawURLEncoding.EncodeToString(b), nil
}

// Repository side.

// ensureRefreshIndexes: one document per token hash; expired tokens are deleted by Mongo.
func (r *Repository) ensureRefreshIndexes(ctx context.Context) error {
	_, err := r.refreshColl.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "token_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "family_id", Value: 1}}},
		{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
	})
	return err
}

func (r *Repository) CreateRefreshToken(ctx context.Context, t *RefreshToken) error {
	_, err := r.refreshColl.InsertOne(ctx, t)
	return err
}

// ClaimRefreshToken marks an unused, unexpired token as used, atomically: two requests with the
// same token can't both claim it.
func (r *Repository) ClaimRefreshToken(ctx context.Context, hash string, now time.Time) (*RefreshToken, error) {
	var t RefreshToken
	err := r.refreshColl.FindOneAndUpdate(ctx,
		bson.M{"token_hash": hash, "used_at": bson.M{"$exists": false}, "expires_at": bson.M{"$gt": now}},
		bson.M{"$set": bson.M{"used_at": now}},
	).Decode(&t)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) FindRefreshToken(ctx context.Context, hash string) (*RefreshToken, error) {
	var t RefreshToken
	if err := r.refreshColl.FindOne(ctx, bson.M{"token_hash": hash}).Decode(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) RevokeRefreshFamily(ctx context.Context, family primitive.ObjectID) error {
	_, err := r.refreshColl.DeleteMany(ctx, bson.M{"family_id": family})
	return err
}
