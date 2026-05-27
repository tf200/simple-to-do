package list

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	models "tf200/todoapp/internal"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, userID primitive.ObjectID, title string) (*models.List, error) {
	list := &models.List{
		UserID:    userID,
		Title:     title,
		CreatedAt: primitive.NewDateTimeFromTime(time.Now()),
	}
	if err := s.repo.Create(ctx, list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) GetByUser(ctx context.Context, userID primitive.ObjectID) ([]models.List, error) {
	return s.repo.FindByUser(ctx, userID)
}

func (s *Service) GetByID(ctx context.Context, userID, listID primitive.ObjectID) (*models.List, error) {
	list, err := s.repo.FindByID(ctx, listID)
	if err != nil {
		return nil, err
	}
	if list.UserID != userID {
		return nil, errors.New("not found")
	}
	return list, nil
}

func (s *Service) Update(ctx context.Context, userID, listID primitive.ObjectID, title string) error {
	list, err := s.repo.FindByID(ctx, listID)
	if err != nil {
		return err
	}
	if list.UserID != userID {
		return errors.New("not found")
	}
	return s.repo.Update(ctx, listID, bson.M{"title": title})
}

func (s *Service) Delete(ctx context.Context, userID, listID primitive.ObjectID) error {
	list, err := s.repo.FindByID(ctx, listID)
	if err != nil {
		return err
	}
	if list.UserID != userID {
		return errors.New("not found")
	}
	return s.repo.Delete(ctx, listID)
}
