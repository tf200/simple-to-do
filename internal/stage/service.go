// Package stage holds each user's workflow: the steps a task goes through between "to do" and
// "done" (say "To show" or "Waiting on client"), in the user's order.
//
// To do and Done aren't stored: a task with no stage is to do (or done, by its done flag).
// A task in a step carries the step's id in its `stage` field.
package stage

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/bson/primitive"

	models "tf200/todoapp/internal"
)

// MaxStages is how many steps a workflow may have between to do and done.
const MaxStages = 12

// ErrInvalid is a workflow the service won't save; the message says why.
var ErrInvalid = errors.New("invalid workflow")

var (
	idPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
	colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// Get returns the user's steps in order (empty when they have none).
func (s *Service) Get(ctx context.Context, userID primitive.ObjectID) ([]models.Stage, error) {
	stages, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if stages == nil {
		stages = []models.Stage{}
	}
	return stages, nil
}

// Has says whether the user has a step with this id.
func (s *Service) Has(ctx context.Context, userID primitive.ObjectID, id string) (bool, error) {
	stages, err := s.repo.Get(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, st := range stages {
		if st.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// Save replaces the user's workflow with `stages`, in that order. Steps keep their ids (the
// client makes them, so it can move tasks into a new step before this returns); a step
// without one gets a new id. Tasks in a step that's gone move to the step before it in the
// old order, or back to to do.
func (s *Service) Save(ctx context.Context, userID primitive.ObjectID, stages []models.Stage) ([]models.Stage, error) {
	clean, err := Clean(stages)
	if err != nil {
		return nil, err
	}
	old, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, userID, clean); err != nil {
		return nil, err
	}
	for from, to := range Removed(old, clean) {
		if err := s.repo.MoveTasks(ctx, userID, from, to); err != nil {
			return nil, err
		}
	}
	return clean, nil
}

// Clean checks a workflow and tidies it: trimmed names, lower-case colours, ids for new steps.
func Clean(stages []models.Stage) ([]models.Stage, error) {
	if len(stages) > MaxStages {
		return nil, invalid("a workflow can have at most 12 steps")
	}
	seen := map[string]bool{}
	out := make([]models.Stage, 0, len(stages))
	for _, st := range stages {
		st.Name = strings.TrimSpace(st.Name)
		if st.Name == "" {
			return nil, invalid("every step needs a name")
		}
		if utf8.RuneCountInString(st.Name) > 40 {
			return nil, invalid("step names can be at most 40 characters")
		}
		if st.ID == "" {
			st.ID = primitive.NewObjectID().Hex()
		}
		if !idPattern.MatchString(st.ID) {
			return nil, invalid("bad step id")
		}
		if seen[st.ID] {
			return nil, invalid("two steps have the same id")
		}
		seen[st.ID] = true
		if !colorPattern.MatchString(st.Color) {
			return nil, invalid("colours look like #rrggbb")
		}
		st.Color = strings.ToLower(st.Color)
		out = append(out, st)
	}
	return out, nil
}

// Removed maps each step in `old` that isn't in `next` to where its tasks go: the nearest
// step before it (in the old order) that's still there, or "" for to do.
func Removed(old, next []models.Stage) map[string]string {
	kept := map[string]bool{}
	for _, st := range next {
		kept[st.ID] = true
	}
	moves := map[string]string{}
	last := ""
	for _, st := range old {
		if kept[st.ID] {
			last = st.ID
			continue
		}
		moves[st.ID] = last
	}
	return moves
}

func invalid(why string) error {
	return &invalidError{why}
}

type invalidError struct{ why string }

func (e *invalidError) Error() string { return e.why }
func (e *invalidError) Unwrap() error { return ErrInvalid }
