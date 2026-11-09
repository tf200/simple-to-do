package task

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	models "tf200/todoapp/internal"
)

type ListOwnership interface {
	GetByID(ctx context.Context, userID, listID primitive.ObjectID) (*models.List, error)
}

type Service struct {
	repo    *Repository
	listSvc ListOwnership
}

func NewService(repo *Repository, listSvc ListOwnership) *Service {
	return &Service{repo: repo, listSvc: listSvc}
}

func (s *Service) Create(ctx context.Context, userID, listID primitive.ObjectID, title, description string, dueAt *primitive.DateTime) (*models.Task, error) {
	if _, err := s.listSvc.GetByID(ctx, userID, listID); err != nil {
		return nil, errors.New("list not found")
	}

	task := &models.Task{
		ListID:      listID,
		Title:       title,
		Description: description,
		DueAt:       dueAt,
		Done:        false,
		CreatedAt:   primitive.NewDateTimeFromTime(time.Now()),
	}
	if err := s.repo.Create(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *Service) GetByList(ctx context.Context, userID, listID primitive.ObjectID) ([]models.Task, error) {
	if _, err := s.listSvc.GetByID(ctx, userID, listID); err != nil {
		return nil, errors.New("list not found")
	}
	return s.repo.FindByList(ctx, listID)
}

func (s *Service) GetByID(ctx context.Context, userID, taskID primitive.ObjectID) (*models.Task, error) {
	task, err := s.repo.FindByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if _, err := s.listSvc.GetByID(ctx, userID, task.ListID); err != nil {
		return nil, errors.New("not found")
	}
	return task, nil
}

// DueChange says what an update does to a task's due date: nothing, set it, or clear it.
type DueChange struct {
	Set   *primitive.DateTime
	Clear bool
}

func (s *Service) Update(ctx context.Context, userID, taskID primitive.ObjectID, title, description *string, due DueChange, done *bool) error {
	task, err := s.repo.FindByID(ctx, taskID)
	if err != nil {
		return err
	}
	if _, err := s.listSvc.GetByID(ctx, userID, task.ListID); err != nil {
		return errors.New("not found")
	}

	update := bson.M{}
	if title != nil {
		update["title"] = *title
	}
	if description != nil {
		update["description"] = *description
	}
	unset := bson.M{}
	if due.Set != nil {
		update["due_at"] = due.Set
	} else if due.Clear {
		unset["due_at"] = ""
	}
	if done != nil {
		update["done"] = *done
	}
	if len(update) == 0 && len(unset) == 0 {
		return nil
	}

	return s.repo.Update(ctx, taskID, update, unset)
}

func (s *Service) Delete(ctx context.Context, userID, taskID primitive.ObjectID) error {
	task, err := s.repo.FindByID(ctx, taskID)
	if err != nil {
		return err
	}
	if _, err := s.listSvc.GetByID(ctx, userID, task.ListID); err != nil {
		return errors.New("not found")
	}
	return s.repo.Delete(ctx, taskID)
}
