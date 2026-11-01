package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo      *Repository
	jwtSecret []byte
}

func NewService(repo *Repository, jwtSecret string) *Service {
	return &Service{
		repo:      repo,
		jwtSecret: []byte(jwtSecret),
	}
}

func HashAPIToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) Login(email, password string) (*LoginResponse, error) {
	user, err := s.repo.FindByEmail(context.Background(), email)
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   user.ID.Hex(),
		"email": user.Email,
		"exp":   time.Now().Add(72 * time.Hour).Unix(),
	})

	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, err
	}

	return &LoginResponse{Token: tokenString}, nil
}

func (s *Service) CreateAPIToken(ctx context.Context, userID primitive.ObjectID, name string) (*CreateAPITokenResponse, error) {
	rawToken, err := generateAPIToken()
	if err != nil {
		return nil, err
	}

	token := &APIToken{
		UserID:    userID,
		Name:      name,
		TokenHash: HashAPIToken(rawToken),
	}
	if err := s.repo.CreateAPIToken(ctx, token); err != nil {
		return nil, err
	}

	return &CreateAPITokenResponse{
		ID:        token.ID,
		Name:      token.Name,
		Token:     rawToken,
		CreatedAt: token.CreatedAt,
	}, nil
}

func (s *Service) ListAPITokens(ctx context.Context, userID primitive.ObjectID) ([]APITokenResponse, error) {
	tokens, err := s.repo.FindActiveAPITokensByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	resp := make([]APITokenResponse, 0, len(tokens))
	for _, token := range tokens {
		resp = append(resp, APITokenResponse{
			ID:        token.ID,
			Name:      token.Name,
			CreatedAt: token.CreatedAt,
		})
	}
	return resp, nil
}

func (s *Service) RevokeAPIToken(ctx context.Context, userID, tokenID primitive.ObjectID) error {
	return s.repo.RevokeAPIToken(ctx, userID, tokenID)
}

func generateAPIToken() (string, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return "todoapp_" + base64.RawURLEncoding.EncodeToString(randomBytes), nil
}
