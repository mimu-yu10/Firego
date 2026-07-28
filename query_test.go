package firego

import (
	"context"
	"errors"
	"testing"
)

func TestWhereFiltersAndDecodes(t *testing.T) {
	store := newFakeStore()
	store.docs["users/abc"] = map[string]any{"name": "Alice", "Age": 30}
	store.docs["users/def"] = map[string]any{"name": "Bob", "Age": 25}
	ref := newTestRef[testUser](t, store, "users")

	got, err := ref.Where("Age", 30).Documents(context.Background())
	if err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	want := []testUser{{ID: "abc", Name: "Alice", Age: 30}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("Documents() = %+v, want %+v", got, want)
	}
}

func TestWhereResolvesGoFieldNameToFirestoreName(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	if _, err := ref.Where("Name", "Alice").Documents(context.Background()); err != nil {
		t.Fatalf("Documents() error = %v", err)
	}

	if store.lastQueryCollection != "users" {
		t.Errorf("QueryDocuments collection = %q, want %q", store.lastQueryCollection, "users")
	}
	if len(store.lastQueryFilters) != 1 {
		t.Fatalf("QueryDocuments filters = %v, want exactly 1 filter", store.lastQueryFilters)
	}
	// "Name" is the Go field; its FirestoreName is "name" per the
	// `firestore:"name"` tag on testUser.
	if got := store.lastQueryFilters[0].Field; got != "name" {
		t.Errorf("filter field = %q, want %q (the FirestoreName, not the Go field name)", got, "name")
	}
}

func TestWhereChainCombinesFiltersWithAnd(t *testing.T) {
	store := newFakeStore()
	store.docs["users/abc"] = map[string]any{"name": "Alice", "Age": 30}
	store.docs["users/def"] = map[string]any{"name": "Alice", "Age": 25}
	ref := newTestRef[testUser](t, store, "users")

	got, err := ref.Where("Name", "Alice").Where("Age", 30).Documents(context.Background())
	if err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "abc" {
		t.Errorf("Documents() = %+v, want exactly the abc document", got)
	}
	if len(store.lastQueryFilters) != 2 {
		t.Fatalf("QueryDocuments filters = %v, want exactly 2 filters", store.lastQueryFilters)
	}
}

func TestQueryIsImmutable(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	base := ref.Where("Name", "Alice")
	if _, err := base.Where("Age", 30).Documents(context.Background()); err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	if len(store.lastQueryFilters) != 2 {
		t.Fatalf("first branch filters = %v, want exactly 2 filters", store.lastQueryFilters)
	}

	// Reusing base for a second, different filter must not see the first
	// branch's Age filter leak in.
	if _, err := base.Where("Age", 99).Documents(context.Background()); err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	if len(store.lastQueryFilters) != 2 {
		t.Fatalf("second branch filters = %v, want exactly 2 filters", store.lastQueryFilters)
	}
	if got := store.lastQueryFilters[1].Value; got != 99 {
		t.Errorf("second branch Age filter = %v, want 99 (base must be unmodified by the first branch)", got)
	}
}

func TestDocumentsRejectsUnknownField(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	_, err := ref.Where("Nickname", "Al").Documents(context.Background())
	if !errors.Is(err, ErrUnknownField) {
		t.Fatalf("Documents() error = %v, want errors.Is(err, ErrUnknownField)", err)
	}
}

func TestDocumentsRejectsIDField(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	_, err := ref.Where("ID", "abc").Documents(context.Background())
	if !errors.Is(err, ErrIDFieldNotQueryable) {
		t.Fatalf("Documents() error = %v, want errors.Is(err, ErrIDFieldNotQueryable)", err)
	}
}

func TestDocumentsPropagatesStoreError(t *testing.T) {
	store := newFakeStore()
	store.queryErr = errors.New("boom")
	ref := newTestRef[testUser](t, store, "users")

	if _, err := ref.Where("Age", 30).Documents(context.Background()); err == nil {
		t.Fatal("Documents() error = nil, want error")
	}
}

