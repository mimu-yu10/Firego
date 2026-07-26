package firego

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/mimu-y10/firego/client"
	"github.com/mimu-y10/firego/codec"
	"github.com/mimu-y10/firego/schema"
)

// ErrNoIDField is returned by (*CollectionRef[T]).Set when the model has no
// field tagged firego:"id".
var ErrNoIDField = errors.New("firego: model has no ID field")

// ErrEmptyID is returned by (*CollectionRef[T]).Set when the model's ID
// field is empty.
var ErrEmptyID = errors.New("firego: ID field is empty")

// ErrInvalidID is returned by Get and Set when id contains a "/". Firestore
// reserves "/" as the separator between a collection and its documents, so
// passing it through unvalidated could address a different document (or an
// invalid path) than the caller intended.
var ErrInvalidID = errors.New(`firego: document ID must not contain "/"`)

// validateID reports an error if id is not a single, legal Firestore
// document-ID path segment.
func validateID(id string) error {
	if strings.Contains(id, "/") {
		return ErrInvalidID
	}
	return nil
}

// docStore is the subset of *client.Client that CollectionRef needs. Its
// purpose is testability: CollectionRef's orchestration logic (encode and
// decode, ID injection, error propagation) can be unit tested against a
// fake implementing this interface, without a live Firestore connection.
type docStore interface {
	GetDocument(ctx context.Context, collection, id string) (map[string]any, error)
	SetDocument(ctx context.Context, collection, id string, data map[string]any) error
}

var _ docStore = (*client.Client)(nil)

// CollectionRef is a type-safe reference to a Firestore collection whose
// documents map to Go values of type T. Create one with Collection.
//
// A CollectionRef holds no mutable state after construction, so it is safe
// for concurrent use.
type CollectionRef[T any] struct {
	store  docStore
	schema *schema.Schema
	codec  codec.Codec
}

// Collection returns a reference to the Firestore collection named name,
// whose documents map to Go values of type T. T must be a struct type, not
// a pointer — pass User, not *User.
func Collection[T any](c *client.Client, name string) (*CollectionRef[T], error) {
	if t := reflect.TypeFor[T](); t.Kind() == reflect.Pointer {
		return nil, fmt.Errorf("firego: Collection[T]: T must not be a pointer type, got %s", t)
	}

	s, err := client.Schema[T](c, name)
	if err != nil {
		return nil, err
	}

	return &CollectionRef[T]{
		store:  c,
		schema: s,
		codec:  codec.New(s),
	}, nil
}

// Get reads the document with the given ID and decodes it into a T. If the
// model declares an ID field, it is populated with id.
//
// If the document does not exist, Get returns an error satisfying
// errors.Is(err, ErrNotFound).
func (r *CollectionRef[T]) Get(ctx context.Context, id string) (T, error) {
	var v T

	if err := validateID(id); err != nil {
		return v, fmt.Errorf("firego: get %s/%s: %w", r.schema.Collection, id, err)
	}

	data, err := r.store.GetDocument(ctx, r.schema.Collection, id)
	if err != nil {
		return v, fmt.Errorf("firego: get %s/%s: %w", r.schema.Collection, id, err)
	}
	if err := r.codec.Decode(data, &v); err != nil {
		return v, fmt.Errorf("firego: get %s/%s: decode: %w", r.schema.Collection, id, err)
	}
	if err := r.codec.SetID(&v, id); err != nil {
		return v, fmt.Errorf("firego: get %s/%s: set id: %w", r.schema.Collection, id, err)
	}
	return v, nil
}

// Set writes v to the document identified by v's ID field, creating it if
// it does not already exist or overwriting it if it does.
//
// The model must declare an ID field (see the firego:"id" struct tag), and
// v's ID field must be non-empty — Firego does not auto-generate document
// IDs. Set returns an error satisfying errors.Is(err, ErrNoIDField) or
// errors.Is(err, ErrEmptyID) if either precondition isn't met.
func (r *CollectionRef[T]) Set(ctx context.Context, v T) error {
	if r.schema.IDField == nil {
		return fmt.Errorf("firego: %s: %w", r.schema.Name, ErrNoIDField)
	}

	id, err := r.codec.ID(v)
	if err != nil {
		return fmt.Errorf("firego: set %s: %w", r.schema.Collection, err)
	}
	if id == "" {
		return fmt.Errorf("firego: %s.%s: %w", r.schema.Name, r.schema.IDField.Name, ErrEmptyID)
	}
	if err := validateID(id); err != nil {
		return fmt.Errorf("firego: set %s/%s: %w", r.schema.Collection, id, err)
	}

	data, err := r.codec.Encode(v)
	if err != nil {
		return fmt.Errorf("firego: set %s/%s: encode: %w", r.schema.Collection, id, err)
	}
	if err := r.store.SetDocument(ctx, r.schema.Collection, id, data); err != nil {
		return fmt.Errorf("firego: set %s/%s: %w", r.schema.Collection, id, err)
	}
	return nil
}
