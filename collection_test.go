package firego

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/mimu-y10/firego/codec"
	"github.com/mimu-y10/firego/internal/metadata"
	"github.com/mimu-y10/firego/query"
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

	getErr   error // when set, getDocument always returns this error
	setErr   error // when set, setDocument always returns this error
	createErr error // when set, createDocument always returns this error
	deleteErr error // when set, deleteDocument always returns this error
	queryErr  error // when set, queryDocuments always returns this error

	lastSetCollection string
	lastSetID         string
	lastSetData       map[string]any

	lastQueryCollection string
	lastQueryFilters    []query.Filter
}

func newFakeStore() *fakeStore {
	return &fakeStore{docs: make(map[string]map[string]any)}
}

func (f *fakeStore) key(collection, id string) string { return collection + "/" + id }

func (f *fakeStore) getDocument(_ context.Context, collection, id string) (map[string]any, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	data, ok := f.docs[f.key(collection, id)]
	if !ok {
		return nil, ErrNotFound
	}
	return data, nil
}

func (f *fakeStore) setDocument(_ context.Context, collection, id string, data map[string]any) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.lastSetCollection = collection
	f.lastSetID = id
	f.lastSetData = data
	f.docs[f.key(collection, id)] = data
	return nil
}

func (f *fakeStore) createDocument(_ context.Context, collection, id string, data map[string]any) error {
	if f.createErr != nil {
		return f.createErr
	}
	key := f.key(collection, id)
	if _, exists := f.docs[key]; exists {
		return ErrAlreadyExists
	}
	f.docs[key] = data
	return nil
}

func (f *fakeStore) deleteDocument(_ context.Context, collection, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.docs, f.key(collection, id))
	return nil
}

func (f *fakeStore) queryDocuments(_ context.Context, collection string, filters []query.Filter) ([]document, error) {
	f.lastQueryCollection = collection
	f.lastQueryFilters = filters
	if f.queryErr != nil {
		return nil, f.queryErr
	}

	prefix := collection + "/"
	var docs []document
	for key, data := range f.docs {
		id, ok := strings.CutPrefix(key, prefix)
		if !ok {
			continue
		}
		if matchesAllFilters(data, filters) {
			docs = append(docs, document{ID: id, Data: data})
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	return docs, nil
}

func matchesAllFilters(data map[string]any, filters []query.Filter) bool {
	for _, f := range filters {
		if data[f.Field] != f.Value {
			return false
		}
	}
	return true
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
	client, err := NewClientFromFirestore(&firestore.Client{})
	if err != nil {
		t.Fatalf("NewClientFromFirestore() error = %v", err)
	}
	if _, err := Collection[*testUser](client, "users"); err == nil {
		t.Fatal("Collection[*testUser]() error = nil, want error")
	}
}

func TestCollectionRejectsInvalidClient(t *testing.T) {
	tests := []struct {
		name   string
		client *Client
	}{
		{name: "nil", client: nil},
		{name: "zero value", client: &Client{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Collection[testUser](tt.client, "users")
			if !errors.Is(err, ErrInvalidClient) {
				t.Fatalf("Collection() error = %v, want errors.Is(err, ErrInvalidClient)", err)
			}
		})
	}
}

func TestCollectionRejectsInvalidPath(t *testing.T) {
	client, err := NewClientFromFirestore(&firestore.Client{})
	if err != nil {
		t.Fatalf("NewClientFromFirestore() error = %v", err)
	}

	for _, path := range []string{"", "/", "/users", "users/", "users//posts", "users/alice"} {
		t.Run(path, func(t *testing.T) {
			_, err := Collection[testUser](client, path)
			if !errors.Is(err, ErrInvalidCollectionPath) {
				t.Fatalf("Collection(%q) error = %v, want errors.Is(err, ErrInvalidCollectionPath)", path, err)
			}
		})
	}
}

func TestValidateCollectionPathAcceptsCollectionPaths(t *testing.T) {
	for _, path := range []string{"users", "users/alice/posts"} {
		t.Run(path, func(t *testing.T) {
			if err := validateCollectionPath(path); err != nil {
				t.Errorf("validateCollectionPath(%q) error = %v", path, err)
			}
		})
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
		t.Errorf("setDocument collection = %q, want %q", store.lastSetCollection, "users")
	}
	if store.lastSetID != "abc" {
		t.Errorf("setDocument id = %q, want %q", store.lastSetID, "abc")
	}
	// Only Name and Age should reach the store — the ID field is never part
	// of the document body.
	if len(store.lastSetData) != 2 {
		t.Fatalf("setDocument data = %v, want exactly 2 fields (name, Age)", store.lastSetData)
	}
	if store.lastSetData["name"] != "Alice" {
		t.Errorf("setDocument data[name] = %v, want Alice", store.lastSetData["name"])
	}
	if store.lastSetData["Age"] != 30 {
		t.Errorf("setDocument data[Age] = %v, want 30", store.lastSetData["Age"])
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

func TestCreateWritesNewDocument(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	want := testUser{ID: "abc", Name: "Alice", Age: 30}
	if err := ref.Create(context.Background(), want); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := ref.Get(context.Background(), "abc")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != want {
		t.Errorf("created document = %+v, want %+v", got, want)
	}
}

func TestCreateRejectsExistingDocument(t *testing.T) {
	store := newFakeStore()
	store.docs["users/abc"] = map[string]any{"name": "Original", "Age": 10}
	ref := newTestRef[testUser](t, store, "users")

	err := ref.Create(context.Background(), testUser{ID: "abc", Name: "Replacement", Age: 30})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("Create() error = %v, want errors.Is(err, ErrAlreadyExists)", err)
	}
	if got := store.docs["users/abc"]["name"]; got != "Original" {
		t.Errorf("existing document name = %v, want Original", got)
	}
}

func TestCreateRejectsInvalidID(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	err := ref.Create(context.Background(), testUser{ID: "a/b", Name: "Alice"})
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("Create() error = %v, want errors.Is(err, ErrInvalidID)", err)
	}
	if len(store.docs) != 0 {
		t.Errorf("Create() wrote to the store despite the invalid ID: %v", store.docs)
	}
}

func TestDeleteRemovesDocument(t *testing.T) {
	store := newFakeStore()
	store.docs["users/abc"] = map[string]any{"name": "Alice"}
	ref := newTestRef[testUser](t, store, "users")

	if err := ref.Delete(context.Background(), "abc"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, exists := store.docs["users/abc"]; exists {
		t.Error("Delete() left the document in the store")
	}
}

func TestDeleteMissingDocumentSucceeds(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	if err := ref.Delete(context.Background(), "missing"); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
}

func TestDeleteRejectsInvalidIDs(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	tests := []struct {
		name string
		id   string
		want error
	}{
		{name: "empty", id: "", want: ErrEmptyID},
		{name: "slash", id: "a/b", want: ErrInvalidID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ref.Delete(context.Background(), tt.id)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Delete(%q) error = %v, want errors.Is(err, %v)", tt.id, err, tt.want)
			}
		})
	}
}

func TestDeletePropagatesStoreError(t *testing.T) {
	store := newFakeStore()
	store.deleteErr = errors.New("delete failed")
	ref := newTestRef[testUser](t, store, "users")

	if err := ref.Delete(context.Background(), "abc"); err == nil {
		t.Fatal("Delete() error = nil, want error")
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
