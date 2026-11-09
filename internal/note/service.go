package note

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	models "tf200/todoapp/internal"
)

// ErrNotFound means the note doesn't exist or belongs to someone else.
var ErrNotFound = errors.New("note not found")

type Service struct {
	repo *Repository
	// Runs before a note is deleted (its attachments go first); an error stops the delete.
	beforeDelete func(ctx context.Context, ids []primitive.ObjectID) error
}

// BeforeDelete sets what runs before a note is deleted.
func (s *Service) BeforeDelete(f func(ctx context.Context, ids []primitive.ObjectID) error) {
	s.beforeDelete = f
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, userID primitive.ObjectID, title, body string) (*models.Note, error) {
	now := primitive.NewDateTimeFromTime(time.Now())
	note := &models.Note{
		UserID:    userID,
		Title:     title,
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, note); err != nil {
		return nil, err
	}
	return note, nil
}

func (s *Service) GetByUser(ctx context.Context, userID primitive.ObjectID) ([]models.Note, error) {
	return s.repo.FindByUser(ctx, userID)
}

func (s *Service) GetByID(ctx context.Context, userID, id primitive.ObjectID) (*models.Note, error) {
	note, err := s.repo.FindOwned(ctx, userID, id)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return note, err
}

// Update changes the given fields and returns the saved note. Nil fields stay as they are.
func (s *Service) Update(ctx context.Context, userID, id primitive.ObjectID, title, body *string, pinned *bool) (*models.Note, error) {
	update := bson.M{}
	if title != nil {
		update["title"] = *title
	}
	if body != nil {
		update["body"] = *body
	}
	if pinned != nil {
		update["pinned"] = *pinned
	}
	// Pinning is not an edit: only content changes move a note to the top.
	if title != nil || body != nil {
		update["updated_at"] = primitive.NewDateTimeFromTime(time.Now())
	}
	if len(update) > 0 {
		err := s.repo.Update(ctx, userID, id, update)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
	}
	return s.GetByID(ctx, userID, id)
}

func (s *Service) Delete(ctx context.Context, userID, id primitive.ObjectID) error {
	if s.beforeDelete != nil {
		if _, err := s.GetByID(ctx, userID, id); err != nil {
			return err
		}
		if err := s.beforeDelete(ctx, []primitive.ObjectID{id}); err != nil {
			return err
		}
	}
	err := s.repo.Delete(ctx, userID, id)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	return err
}
