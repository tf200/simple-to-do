package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
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

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email already registered")
)

// NormalizeEmail makes emails case-insensitive: "Taha@X.com" and "taha@x.com" are one account.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *Service) Login(email, password string) (*LoginResponse, error) {
	user, err := s.repo.FindByEmail(context.Background(), NormalizeEmail(email))
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.issueToken(user)
}

func (s *Service) Register(ctx context.Context, email, password string) (*LoginResponse, error) {
	email = NormalizeEmail(email)
	if _, err := s.repo.FindByEmail(ctx, email); err == nil {
		return nil, ErrEmailTaken
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &User{ID: primitive.NewObjectID(), Email: email, Password: string(hash)}
	if err := s.repo.Create(ctx, user); err != nil {
		// The unique index catches a concurrent registration of the same email.
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}

	return s.issueToken(user)
}

func (s *Service) Me(ctx context.Context, userID primitive.ObjectID) (*UserResponse, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &UserResponse{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: primitive.NewDateTimeFromTime(user.CreatedAt),
	}, nil
}

func (s *Service) issueToken(user *User) (*LoginResponse, error) {
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
