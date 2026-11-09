package attachment

import (
	"context"
	"errors"
	"fmt"
	"log"
	"mime"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	models "tf200/todoapp/internal"
)

const (
	KindTask = "task"
	KindNote = "note"

	StatusPending = "pending"
	StatusReady   = "ready"

	// MaxFileSize is the largest file one attachment may be.
	MaxFileSize int64 = 25 << 20
	// UserQuota is how much one user may store in all.
	UserQuota int64 = 1 << 30
	// MaxPerParent is how many files one task or note may have.
	MaxPerParent = 50

	// UploadTTL is how long an upload link works; DownloadTTL the same for a download link.
	UploadTTL   = 15 * time.Minute
	DownloadTTL = 5 * time.Minute
	// An upload not confirmed this long after it started is given up on and cleaned away.
	abandonAfter = time.Hour
)

var (
	ErrNotFound    = errors.New("not found")
	ErrDisabled    = errors.New("attachments are not set up on this server")
	ErrTooLarge    = fmt.Errorf("files can be at most %d MB", MaxFileSize>>20)
	ErrEmpty       = errors.New("the file is empty")
	ErrQuota       = fmt.Errorf("storage is full: each account can keep %d GB of attachments", UserQuota>>30)
	ErrTooMany     = fmt.Errorf("at most %d attachments per task or note", MaxPerParent)
	ErrBadName     = errors.New("invalid file name")
	ErrBadMime     = errors.New("invalid content type")
	ErrNotUploaded = errors.New("the file hasn't been uploaded yet")
	ErrSizeChanged = errors.New("the uploaded file isn't the size that was announced")
)

// Parents checks that a task or note exists and belongs to the user (ErrNotFound if not).
type Parents interface {
	Owned(ctx context.Context, userID primitive.ObjectID, kind string, id primitive.ObjectID) error
}

type Service struct {
	repo    *Repository
	store   Store
	parents Parents
	enabled bool
	now     func() time.Time
}

// NewService: `enabled` false keeps uploads and downloads off (the store may still clean up).
func NewService(repo *Repository, store Store, parents Parents, enabled bool) *Service {
	return &Service{repo: repo, store: store, parents: parents, enabled: enabled, now: time.Now}
}

// Upload says how to send the file: PUT the bytes to URL with Headers, before ExpiresAt.
type Upload struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// Start records a pending attachment and returns a link to upload it with.
func (s *Service) Start(ctx context.Context, userID primitive.ObjectID, kind string, parentID primitive.ObjectID, name string, size int64, contentType string) (*models.Attachment, *Upload, error) {
	if !s.enabled {
		return nil, nil, ErrDisabled
	}
	name, err := CleanName(name)
	if err != nil {
		return nil, nil, err
	}
	contentType, err = cleanMime(contentType)
	if err != nil {
		return nil, nil, err
	}
	switch {
	case size <= 0:
		return nil, nil, ErrEmpty
	case size > MaxFileSize:
		return nil, nil, ErrTooLarge
	}
	if err := s.parents.Owned(ctx, userID, kind, parentID); err != nil {
		return nil, nil, err
	}
	count, err := s.repo.CountByParent(ctx, kind, parentID)
	if err != nil {
		return nil, nil, err
	}
	if count >= MaxPerParent {
		return nil, nil, ErrTooMany
	}
	used, err := s.repo.UsedBytes(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if used+size > UserQuota {
		return nil, nil, ErrQuota
	}

	id := primitive.NewObjectID()
	a := &models.Attachment{
		ID:         id,
		UserID:     userID,
		ParentKind: kind,
		ParentID:   parentID,
		Name:       name,
		Mime:       contentType,
		Size:       size,
		// Only ids in the key: the name the user gave stays in the database.
		Key:       fmt.Sprintf("users/%s/%s", userID.Hex(), id.Hex()),
		Status:    StatusPending,
		CreatedAt: primitive.NewDateTimeFromTime(s.now()),
	}
	link, headers, err := s.store.PresignPut(ctx, a.Key, size, contentType, UploadTTL)
	if err != nil {
		return nil, nil, err
	}
	if err := s.repo.Create(ctx, a); err != nil {
		return nil, nil, err
	}
	return a, &Upload{URL: link, Method: "PUT", Headers: headers, ExpiresAt: s.now().Add(UploadTTL)}, nil
}

// Complete confirms an upload: the file must be in the store, at the announced size.
func (s *Service) Complete(ctx context.Context, userID, id primitive.ObjectID) (*models.Attachment, error) {
	a, err := s.owned(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if a.Status == StatusReady {
		return a, nil
	}
	// The task or note may have been deleted while the file was on its way.
	if err := s.parents.Owned(ctx, userID, a.ParentKind, a.ParentID); err != nil {
		s.remove(ctx, a)
		return nil, err
	}
	size, err := s.store.Size(ctx, a.Key)
	if errors.Is(err, ErrNoObject) {
		return nil, ErrNotUploaded
	}
	if err != nil {
		return nil, err
	}
	if size != a.Size {
		s.remove(ctx, a)
		return nil, ErrSizeChanged
	}
	if _, err := s.repo.MarkReady(ctx, a.ID); err != nil {
		return nil, err
	}
	a.Status = StatusReady
	return a, nil
}

// List returns the uploaded files of a task or note, oldest first.
func (s *Service) List(ctx context.Context, userID primitive.ObjectID, kind string, parentID primitive.ObjectID) ([]models.Attachment, error) {
	if err := s.parents.Owned(ctx, userID, kind, parentID); err != nil {
		return nil, err
	}
	return s.repo.FindByParent(ctx, kind, parentID, StatusReady)
}

// Download is a short-lived link to the file.
type Download struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Service) Download(ctx context.Context, userID, id primitive.ObjectID) (*Download, error) {
	if !s.enabled {
		return nil, ErrDisabled
	}
	a, err := s.owned(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if a.Status != StatusReady {
		return nil, ErrNotUploaded
	}
	link, err := s.store.PresignGet(ctx, a.Key, a.Name, a.Mime, DownloadTTL)
	if err != nil {
		return nil, err
	}
	return &Download{URL: link, ExpiresAt: s.now().Add(DownloadTTL)}, nil
}

func (s *Service) Delete(ctx context.Context, userID, id primitive.ObjectID) error {
	a, err := s.owned(ctx, userID, id)
	if err != nil {
		return err
	}
	if err := s.store.Delete(ctx, a.Key); err != nil {
		return err
	}
	return s.repo.Delete(ctx, a.ID)
}

// DeleteParents removes every file of the given tasks or notes. Called before they're deleted:
// if the store can't be reached, the error stops the delete, so no files are left behind.
func (s *Service) DeleteParents(ctx context.Context, kind string, ids []primitive.ObjectID) error {
	if len(ids) == 0 {
		return nil
	}
	all, err := s.repo.FindByParents(ctx, kind, ids)
	if err != nil {
		return err
	}
	for i := range all {
		if err := s.store.Delete(ctx, all[i].Key); err != nil {
			return err
		}
		if err := s.repo.Delete(ctx, all[i].ID); err != nil {
			return err
		}
	}
	return nil
}

// Sweep cleans away uploads that were started but never confirmed.
func (s *Service) Sweep(ctx context.Context) (int, error) {
	stale, err := s.repo.FindPendingBefore(ctx, s.now().Add(-abandonAfter))
	if err != nil {
		return 0, err
	}
	removed := 0
	for i := range stale {
		if s.remove(ctx, &stale[i]) == nil {
			removed++
		}
	}
	return removed, nil
}

// SweepEvery runs Sweep on a timer until ctx ends.
func (s *Service) SweepEvery(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := s.Sweep(ctx); err != nil {
				log.Printf("attachments: sweep failed: %v", err)
			} else if n > 0 {
				log.Printf("attachments: cleaned away %d abandoned uploads", n)
			}
		}
	}
}

func (s *Service) owned(ctx context.Context, userID, id primitive.ObjectID) (*models.Attachment, error) {
	a, err := s.repo.FindOwned(ctx, userID, id)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return a, err
}

// remove deletes the object and then the record; errors are logged, the record stays on failure.
func (s *Service) remove(ctx context.Context, a *models.Attachment) error {
	if err := s.store.Delete(ctx, a.Key); err != nil {
		log.Printf("attachments: could not delete %s: %v", a.Key, err)
		return err
	}
	return s.repo.Delete(ctx, a.ID)
}

// CleanName keeps the last path element of a file name, without control characters, and
// checks it's something a file could be called.
func CleanName(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || name == "/" || !utf8.ValidString(name) || len(name) > 255 {
		return "", ErrBadName
	}
	return name, nil
}

// cleanMime normalises a content type; empty means a generic binary file.
func cleanMime(contentType string) (string, error) {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return "application/octet-stream", nil
	}
	media, params, err := mime.ParseMediaType(contentType)
	if err != nil || len(contentType) > 200 || !strings.Contains(media, "/") {
		return "", ErrBadMime
	}
	return mime.FormatMediaType(media, params), nil
}
