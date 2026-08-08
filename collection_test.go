package firego

import (
	"context"
	"errors"
	"testing"

	"github.com/mimu-y10/firego/codec"
	"github.com/mimu-y10/firego/internal/metadata"
)

type testUser struct {
	ID   string `firego:"id" firestore:"-"`
	Name string `firestore:"name"`
	Age  int
}

// testItem declares no ID field, exercising Set's ErrNoIDField path.
type testItem struct {
	Name string `firestore:"name"`
}

// fakeStore is an in-memory docStore. It lets CollectionRef's orchestration
// logic (encode/decode, ID injection, error propagation) be unit tested
// without a live Firestore connection.
type fakeStore struct {
	docs map[string]map[string]any

	getErr error // when set, GetDocument always returns this error
	setErr error // when set, SetDocument always returns this error

	lastSetCollection string
	lastSetID         string
	lastSetData       map[string]any
}

func newFakeStore() *fakeStore {
	return &fakeStore{docs: make(map[string]map[string]any)}
}

func (f *fakeStore) key(collection, id string) string { return collection + "/" + id }

func (f *fakeStore) GetDocument(_ context.Context, collection, id string) (map[string]any, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	data, ok := f.docs[f.key(collection, id)]
	if !ok {
		return nil, ErrNotFound
	}
	return data, nil
}

func (f *fakeStore) SetDocument(_ context.Context, collection, id string, data map[string]any) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.lastSetCollection = collection
	f.lastSetID = id
	f.lastSetData = data
	f.docs[f.key(collection, id)] = data
	return nil
}

var _ docStore = (*fakeStore)(nil)

func newTestRef[T any](t *testing.T, store docStore, collection string) *CollectionRef[T] {
	t.Helper()
	s, err := metadata.Parse[T](collection)
	if err != nil {
		t.Fatalf("metadata.Parse() error = %v", err)
	}
	return &CollectionRef[T]{store: store, schema: s, codec: codec.New(s)}
}

func TestCollectionRejectsPointerType(t *testing.T) {
	if _, err := Collection[*testUser](nil, "users"); err == nil {
		t.Fatal("Collection[*testUser]() error = nil, want error")
	}
}

func TestGetDecodesAndInjectsID(t *testing.T) {
	store := newFakeStore()
	store.docs["users/abc"] = map[string]any{"name": "Alice", "Age": int64(30)}
	ref := newTestRef[testUser](t, store, "users")

	got, err := ref.Get(context.Background(), "abc")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	want := testUser{ID: "abc", Name: "Alice", Age: 30}
	if got != want {
		t.Errorf("Get() = %+v, want %+v", got, want)
	}
}

func TestGetPropagatesNotFound(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	_, err := ref.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want errors.Is(err, firego.ErrNotFound)", err)
	}
}

func TestGetWrapsDecodeError(t *testing.T) {
	store := newFakeStore()
	store.docs["users/abc"] = map[string]any{"Age": "not a number"}
	ref := newTestRef[testUser](t, store, "users")

	if _, err := ref.Get(context.Background(), "abc"); err == nil {
		t.Fatal("Get() error = nil, want error")
	}
}

func TestSetRejectsModelWithoutIDField(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testItem](t, store, "items")

	err := ref.Set(context.Background(), testItem{Name: "widget"})
	if !errors.Is(err, ErrNoIDField) {
		t.Fatalf("Set() error = %v, want errors.Is(err, ErrNoIDField)", err)
	}
	if len(store.docs) != 0 {
		t.Errorf("Set() wrote to the store despite the missing ID field: %v", store.docs)
	}
}

func TestSetRejectsEmptyID(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	err := ref.Set(context.Background(), testUser{Name: "Bob"})
	if !errors.Is(err, ErrEmptyID) {
		t.Fatalf("Set() error = %v, want errors.Is(err, ErrEmptyID)", err)
	}
	if len(store.docs) != 0 {
		t.Errorf("Set() wrote to the store despite the empty ID: %v", store.docs)
	}
}

func TestSetWritesEncodedData(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	if err := ref.Set(context.Background(), testUser{ID: "abc", Name: "Alice", Age: 30}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if store.lastSetCollection != "users" {
		t.Errorf("SetDocument collection = %q, want %q", store.lastSetCollection, "users")
	}
	if store.lastSetID != "abc" {
		t.Errorf("SetDocument id = %q, want %q", store.lastSetID, "abc")
	}
	// Only Name and Age should reach the store — the ID field is never part
	// of the document body.
	if len(store.lastSetData) != 2 {
		t.Fatalf("SetDocument data = %v, want exactly 2 fields (name, Age)", store.lastSetData)
	}
	if store.lastSetData["name"] != "Alice" {
		t.Errorf("SetDocument data[name] = %v, want Alice", store.lastSetData["name"])
	}
	if store.lastSetData["Age"] != 30 {
		t.Errorf("SetDocument data[Age] = %v, want 30", store.lastSetData["Age"])
	}
}

func TestSetThenGetRoundTrip(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	want := testUser{ID: "abc", Name: "Alice", Age: 30}
	if err := ref.Set(context.Background(), want); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, err := ref.Get(context.Background(), "abc")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestGetRejectsIDContainingSlash(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	_, err := ref.Get(context.Background(), "a/b")
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("Get() error = %v, want errors.Is(err, ErrInvalidID)", err)
	}
}

func TestSetRejectsIDContainingSlash(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	err := ref.Set(context.Background(), testUser{ID: "a/b", Name: "Alice"})
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("Set() error = %v, want errors.Is(err, ErrInvalidID)", err)
	}
	if len(store.docs) != 0 {
		t.Errorf("Set() wrote to the store despite the invalid ID: %v", store.docs)
	}
}

func TestGetRejectsEmptyID(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	_, err := ref.Get(context.Background(), "")
	if !errors.Is(err, ErrEmptyID) {
		t.Fatalf("Get() error = %v, want errors.Is(err, ErrEmptyID)", err)
	}
}
