package metadata

import (
	"strings"
	"sync"
	"testing"

	"github.com/mimu-y10/firego/schema"
)

func TestGetCachesSchema(t *testing.T) {
	r := NewRegistry()

	first, err := Get[testUser](r, "users")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	second, err := Get[testUser](r, "users")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if first != second {
		t.Fatalf("Get() returned different *schema.Schema pointers for the same (T, collection); first = %p, second = %p", first, second)
	}
}

func TestGetDifferentCollectionsAreSeparateEntries(t *testing.T) {
	r := NewRegistry()

	users, err := Get[testUser](r, "users")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	archived, err := Get[testUser](r, "archived_users")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if users == archived {
		t.Fatalf("Get() returned the same *schema.Schema for different collections")
	}
	if users.Collection != "users" {
		t.Errorf("users.Collection = %q, want %q", users.Collection, "users")
	}
	if archived.Collection != "archived_users" {
		t.Errorf("archived.Collection = %q, want %q", archived.Collection, "archived_users")
	}
}

func TestGetPointerAndValueShareCacheEntry(t *testing.T) {
	r := NewRegistry()

	byValue, err := Get[testUser](r, "users")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	byPointer, err := Get[*testUser](r, "users")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if byValue != byPointer {
		t.Fatalf("Get[testUser] and Get[*testUser] did not share a cache entry; byValue = %p, byPointer = %p", byValue, byPointer)
	}
}

func TestGetDoesNotCacheErrors(t *testing.T) {
	r := NewRegistry()

	if _, err := Get[testUser](r, ""); err == nil || !strings.Contains(err.Error(), "collection must not be empty") {
		t.Fatalf("Get() error = %v, want error containing %q", err, "collection must not be empty")
	}

	// A later, valid call for the same type must still succeed — the failed
	// call above must not have poisoned the cache.
	got, err := Get[testUser](r, "users")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Collection != "users" {
		t.Errorf("Collection = %q, want %q", got.Collection, "users")
	}
}

func TestGetConcurrent(t *testing.T) {
	r := NewRegistry()

	const goroutines = 50
	results := make([]*schemaOrError, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			s, err := Get[testUser](r, "users")
			results[i] = &schemaOrError{schema: s, err: err}
		}()
	}
	wg.Wait()

	want := results[0]
	if want.err != nil {
		t.Fatalf("Get() error = %v", want.err)
	}
	for i, got := range results {
		if got.err != nil {
			t.Fatalf("goroutine %d: Get() error = %v", i, got.err)
		}
		if got.schema != want.schema {
			t.Fatalf("goroutine %d: Get() returned %p, want %p (all callers must observe the same cached schema)", i, got.schema, want.schema)
		}
	}
}

type schemaOrError struct {
	schema *schema.Schema
	err    error
}
