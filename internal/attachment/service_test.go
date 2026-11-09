package attachment

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestCleanName(t *testing.T) {
	good := map[string]string{
		"report.pdf":            "report.pdf",
		"  spaced name.txt  ":   "spaced name.txt",
		`C:\Users\me\photo.jpg`: "photo.jpg",
		"/etc/passwd":           "passwd",
		"../../secret":          "secret",
		"tab\there.txt":         "tabhere.txt",
		"émoji 🎉.png":           "émoji 🎉.png",
	}
	for in, want := range good {
		got, err := CleanName(in)
		if err != nil || got != want {
			t.Errorf("CleanName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "   ", ".", "..", "/", "\x00\x01", string(make([]byte, 300))} {
		if got, err := CleanName(in); !errors.Is(err, ErrBadName) {
			t.Errorf("CleanName(%q) = %q, %v; want ErrBadName", in, got, err)
		}
	}
}

func TestCleanMime(t *testing.T) {
	for in, want := range map[string]string{
		"":                          "application/octet-stream",
		"image/PNG":                 "image/png",
		"text/plain; charset=utf-8": "text/plain; charset=utf-8",
	} {
		if got, err := cleanMime(in); err != nil || got != want {
			t.Errorf("cleanMime(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"nonsense", "text/plain; =", "a/b\r\nX-Evil: 1"} {
		if _, err := cleanMime(in); err == nil {
			t.Errorf("cleanMime(%q) accepted", in)
		}
	}
}

func TestContentDisposition(t *testing.T) {
	got := contentDisposition(`résumé "final"; v2.pdf`)
	want := `attachment; filename="r_sum_ _final_; v2.pdf"; filename*=UTF-8''r%C3%A9sum%C3%A9%20%22final%22%3B%20v2.pdf`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// --- service tests: a fake store, a real Mongo (TEST_MONGO_URI, else skipped) ---

type fakeStore struct {
	mu      sync.Mutex
	objects map[string]int64
	failDel bool
}

func (f *fakeStore) PresignPut(_ context.Context, key string, size int64, mime string, _ time.Duration) (string, map[string]string, error) {
	return "https://store/" + key, map[string]string{"Content-Type": mime}, nil
}
func (f *fakeStore) PresignGet(_ context.Context, key, _, _ string, _ time.Duration) (string, error) {
	return "https://store/" + key + "?get", nil
}
func (f *fakeStore) Size(_ context.Context, key string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.objects[key]
	if !ok {
		return 0, ErrNoObject
	}
	return n, nil
}
func (f *fakeStore) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failDel {
		return errors.New("store down")
	}
	delete(f.objects, key)
	return nil
}
func (f *fakeStore) put(key string, n int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = n
}

// fakeParents owns whatever is in the set, for its user.
type fakeParents struct {
	owner primitive.ObjectID
	ids   map[primitive.ObjectID]bool
}

func (p *fakeParents) Owned(_ context.Context, userID primitive.ObjectID, _ string, id primitive.ObjectID) error {
	if userID != p.owner || !p.ids[id] {
		return ErrNotFound
	}
	return nil
}

type fixture struct {
	svc     *Service
	store   *fakeStore
	parents *fakeParents
	user    primitive.ObjectID
	task    primitive.ObjectID
}

func setup(t *testing.T) *fixture {
	t.Helper()
	uri := os.Getenv("TEST_MONGO_URI")
	if uri == "" {
		t.Skip("TEST_MONGO_URI not set")
	}
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	db := client.Database("attachment_test_" + primitive.NewObjectID().Hex())
	t.Cleanup(func() {
		_ = db.Drop(ctx)
		_ = client.Disconnect(ctx)
	})
	repo := NewRepository(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		store: &fakeStore{objects: map[string]int64{}},
		user:  primitive.NewObjectID(),
		task:  primitive.NewObjectID(),
	}
	f.parents = &fakeParents{owner: f.user, ids: map[primitive.ObjectID]bool{f.task: true}}
	f.svc = NewService(repo, f.store, f.parents, true)
	return f
}

func TestUploadFlow(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	a, up, err := f.svc.Start(ctx, f.user, KindTask, f.task, "notes.txt", 5, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if up.Method != "PUT" || up.URL == "" || a.Status != StatusPending {
		t.Fatalf("unexpected start: %+v %+v", a, up)
	}
	// Not listed, not downloadable, not completable until the bytes are there.
	if list, _ := f.svc.List(ctx, f.user, KindTask, f.task); len(list) != 0 {
		t.Fatalf("pending attachment listed: %+v", list)
	}
	if _, err := f.svc.Download(ctx, f.user, a.ID); !errors.Is(err, ErrNotUploaded) {
		t.Fatalf("download of pending: %v", err)
	}
	if _, err := f.svc.Complete(ctx, f.user, a.ID); !errors.Is(err, ErrNotUploaded) {
		t.Fatalf("complete before upload: %v", err)
	}

	f.store.put(a.Key, 5)
	done, err := f.svc.Complete(ctx, f.user, a.ID)
	if err != nil || done.Status != StatusReady {
		t.Fatalf("complete: %+v %v", done, err)
	}
	// Completing twice is fine.
	if _, err := f.svc.Complete(ctx, f.user, a.ID); err != nil {
		t.Fatalf("second complete: %v", err)
	}
	list, err := f.svc.List(ctx, f.user, KindTask, f.task)
	if err != nil || len(list) != 1 || list[0].Name != "notes.txt" {
		t.Fatalf("list: %+v %v", list, err)
	}
	if d, err := f.svc.Download(ctx, f.user, a.ID); err != nil || d.URL == "" {
		t.Fatalf("download: %+v %v", d, err)
	}

	// Someone else sees nothing.
	other := primitive.NewObjectID()
	if _, err := f.svc.Download(ctx, other, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user's download: %v", err)
	}
	if err := f.svc.Delete(ctx, other, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user's delete: %v", err)
	}
	if _, err := f.svc.List(ctx, other, KindTask, f.task); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user's list: %v", err)
	}

	if err := f.svc.Delete(ctx, f.user, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Size(ctx, a.Key); !errors.Is(err, ErrNoObject) {
		t.Fatal("object left in the store after delete")
	}
}

func TestStartRejects(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	cases := []struct {
		name   string
		parent primitive.ObjectID
		size   int64
		file   string
		want   error
	}{
		{"too large", f.task, MaxFileSize + 1, "a.bin", ErrTooLarge},
		{"empty", f.task, 0, "a.bin", ErrEmpty},
		{"bad name", f.task, 10, "..", ErrBadName},
		{"not my task", primitive.NewObjectID(), 10, "a.bin", ErrNotFound},
	}
	for _, c := range cases {
		if _, _, err := f.svc.Start(ctx, f.user, KindTask, c.parent, c.file, c.size, ""); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}

	disabled := NewService(f.svc.repo, f.store, f.parents, false)
	if _, _, err := disabled.Start(ctx, f.user, KindTask, f.task, "a.bin", 10, ""); !errors.Is(err, ErrDisabled) {
		t.Errorf("disabled: %v", err)
	}
}

func TestQuotaAndCount(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	// Pending uploads count towards the quota: fill it with max-size ones.
	n := int(UserQuota / MaxFileSize)
	for i := 0; i < n; i++ {
		parent := primitive.NewObjectID()
		f.parents.ids[parent] = true
		if _, _, err := f.svc.Start(ctx, f.user, KindNote, parent, "big.bin", MaxFileSize, ""); err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}
	if _, _, err := f.svc.Start(ctx, f.user, KindTask, f.task, "one-more.bin", MaxFileSize, ""); !errors.Is(err, ErrQuota) {
		t.Fatalf("over quota: %v", err)
	}

	g := setup(t)
	for i := 0; i < MaxPerParent; i++ {
		if _, _, err := g.svc.Start(ctx, g.user, KindTask, g.task, "f.txt", 1, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := g.svc.Start(ctx, g.user, KindTask, g.task, "f.txt", 1, ""); !errors.Is(err, ErrTooMany) {
		t.Fatalf("too many: %v", err)
	}
}

func TestCompleteChecks(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// A different size than announced: the upload is thrown away.
	a, _, _ := f.svc.Start(ctx, f.user, KindTask, f.task, "a.txt", 10, "")
	f.store.put(a.Key, 11)
	if _, err := f.svc.Complete(ctx, f.user, a.ID); !errors.Is(err, ErrSizeChanged) {
		t.Fatalf("size mismatch: %v", err)
	}
	if _, err := f.store.Size(ctx, a.Key); !errors.Is(err, ErrNoObject) {
		t.Fatal("mismatched upload kept")
	}
	if _, err := f.svc.Complete(ctx, f.user, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("record kept after mismatch: %v", err)
	}

	// The task went away during the upload.
	b, _, _ := f.svc.Start(ctx, f.user, KindTask, f.task, "b.txt", 3, "")
	f.store.put(b.Key, 3)
	delete(f.parents.ids, f.task)
	if _, err := f.svc.Complete(ctx, f.user, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("parent gone: %v", err)
	}
	if _, err := f.store.Size(ctx, b.Key); !errors.Is(err, ErrNoObject) {
		t.Fatal("upload kept after its task was deleted")
	}
}

func TestDeleteParentsAndSweep(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	ready, _, _ := f.svc.Start(ctx, f.user, KindTask, f.task, "a.txt", 2, "")
	f.store.put(ready.Key, 2)
	if _, err := f.svc.Complete(ctx, f.user, ready.ID); err != nil {
		t.Fatal(err)
	}
	pending, _, _ := f.svc.Start(ctx, f.user, KindTask, f.task, "b.txt", 2, "")

	// The store is down: the delete must fail, so the task isn't deleted with files left behind.
	f.store.failDel = true
	if err := f.svc.DeleteParents(ctx, KindTask, []primitive.ObjectID{f.task}); err == nil {
		t.Fatal("DeleteParents succeeded with the store down")
	}
	f.store.failDel = false
	if err := f.svc.DeleteParents(ctx, KindTask, []primitive.ObjectID{f.task}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []primitive.ObjectID{ready.ID, pending.ID} {
		if _, err := f.svc.repo.FindOwned(ctx, f.user, id); !errors.Is(err, mongo.ErrNoDocuments) {
			t.Fatalf("attachment %s survived its task", id.Hex())
		}
	}

	// Sweep: only pending uploads older than an hour go.
	old, _, _ := f.svc.Start(ctx, f.user, KindTask, f.task, "old.txt", 1, "")
	fresh, _, _ := f.svc.Start(ctx, f.user, KindTask, f.task, "fresh.txt", 1, "")
	f.svc.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	newer, _, _ := f.svc.Start(ctx, f.user, KindTask, f.task, "newer.txt", 1, "")
	n, err := f.svc.Sweep(ctx)
	if err != nil || n != 2 {
		t.Fatalf("sweep removed %d (%v), want 2", n, err)
	}
	for _, a := range []primitive.ObjectID{old.ID, fresh.ID} {
		if _, err := f.svc.repo.FindOwned(ctx, f.user, a); !errors.Is(err, mongo.ErrNoDocuments) {
			t.Fatal("stale upload not swept")
		}
	}
	if _, err := f.svc.repo.FindOwned(ctx, f.user, newer.ID); err != nil {
		t.Fatal("recent upload swept")
	}
}
