package firego

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mimu-y10/firego/codec"
	"github.com/mimu-y10/firego/query"
	"github.com/mimu-y10/firego/schema"
)

// ErrNoIDField is returned by (*CollectionRef[T]).Set when the model has no
// field tagged firego:"id".
var ErrNoIDField = errors.New("firego: model has no ID field")

// ErrEmptyID is returned by Get when called with an empty id, and by Set
// when the model's ID field is empty.
var ErrEmptyID = errors.New("firego: ID field is empty")

// ErrInvalidID is returned by Get and Set when id contains a "/". Firestore
// reserves "/" as the separator between a collection and its documents, so
// passing it through unvalidated could address a different document (or an
// invalid path) than the caller intended.
var ErrInvalidID = errors.New(`firego: document ID must not contain "/"`)

// ErrInvalidClient is returned when Collection receives a nil or
// uninitialized Client.
var ErrInvalidClient = errors.New("firego: client is nil or uninitialized")

// ErrInvalidCollectionPath is returned when a collection path is empty,
// contains an empty segment, or does not end at a collection.
var ErrInvalidCollectionPath = errors.New("firego: invalid collection path")

// ErrEmptyUpdates is returned when Update receives no fields to update.
var ErrEmptyUpdates = errors.New("firego: update requires at least one field")

// ErrIDFieldNotWritable is returned when Update targets the model's ID field.
var ErrIDFieldNotWritable = errors.New("firego: ID field cannot be updated")

// ErrInvalidUpdateValue is returned when an update value is incompatible
// with the model field it targets.
var ErrInvalidUpdateValue = errors.New("firego: invalid update value")

// FieldUpdate assigns Value to the model field named Field. Field uses the Go
// struct field name; Firego resolves its Firestore name from the model schema.
type FieldUpdate struct {
	Field string
	Value any
}

var (
	firestoreSentinelType    = reflect.TypeOf(firestore.Delete)
	firestoreArrayUnionType  = reflect.TypeOf(firestore.ArrayUnion())
	firestoreArrayRemoveType = reflect.TypeOf(firestore.ArrayRemove())
	firestoreTransformType   = reflect.TypeOf(firestore.Increment(0))
	timeType                 = reflect.TypeFor[time.Time]()
)

func validateFirestoreSentinel(value any, target reflect.Type) (bool, error) {
	valueType := reflect.TypeOf(value)
	switch valueType {
	case firestoreSentinelType:
		if reflect.DeepEqual(value, firestore.ServerTimestamp) && target != timeType {
			return true, fmt.Errorf("server timestamp requires %s, got %s", timeType, target)
		}
		return true, nil
	case firestoreArrayUnionType, firestoreArrayRemoveType:
		if target.Kind() != reflect.Slice && target.Kind() != reflect.Array {
			return true, fmt.Errorf("array transform requires a slice or array field, got %s", target)
		}
		return true, nil
	case firestoreTransformType:
		if !isNumericType(target) {
			return true, fmt.Errorf("numeric transform requires a numeric field, got %s", target)
		}
		return true, nil
	default:
		return false, nil
	}
}

func isNumericType(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// validateID reports an error if id is not a single, legal Firestore
// document-ID path segment.
func validateID(id string) error {
	if strings.Contains(id, "/") {
		return ErrInvalidID
	}
	return nil
}

func validateCollectionPath(path string) error {
	segments := strings.Split(path, "/")
	if len(segments)%2 == 0 {
		return ErrInvalidCollectionPath
	}
	for _, segment := range segments {
		if segment == "" {
			return ErrInvalidCollectionPath
		}
	}
	return nil
}

// crudStore is the subset of *Client that both CollectionRef and
// TxCollectionRef need for reading and writing individual documents. Its
// purpose is testability: their orchestration logic (encode and decode, ID
// injection, error propagation) can be unit tested against a fake
// implementing this interface, without a live Firestore connection.
type crudStore interface {
	getDocument(ctx context.Context, collection, id string) (map[string]any, error)
	setDocument(ctx context.Context, collection, id string, data map[string]any) error
	createDocument(ctx context.Context, collection, id string, data map[string]any) error
	deleteDocument(ctx context.Context, collection, id string) error
	updateDocument(ctx context.Context, collection, id string, updates []documentUpdate) error
}

// docStore additionally supports queries, which are only available outside
// a transaction.
type docStore interface {
	crudStore
	queryDocuments(ctx context.Context, collection string, filters []query.Filter, orders []query.Order, limit *int, start, end *query.Cursor) ([]document, error)
}

var _ docStore = (*Client)(nil)

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
func Collection[T any](c *Client, name string) (*CollectionRef[T], error) {
	if c == nil || c.firestore == nil || c.registry == nil {
		return nil, ErrInvalidClient
	}
	if err := validateCollectionPath(name); err != nil {
		return nil, fmt.Errorf("firego: collection %q: %w", name, err)
	}
	if t := reflect.TypeFor[T](); t.Kind() == reflect.Pointer {
		return nil, fmt.Errorf("firego: Collection[T]: T must not be a pointer type, got %s", t)
	}

	s, err := schemaFor[T](c, name)
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
// errors.Is(err, ErrNotFound). It returns an error satisfying
// errors.Is(err, ErrEmptyID) or errors.Is(err, ErrInvalidID) if id is empty
// or contains a "/", without making a request.
func (r *CollectionRef[T]) Get(ctx context.Context, id string) (T, error) {
	return getDoc[T](ctx, r.store, r.schema, r.codec, id)
}

// Set writes v to the document identified by v's ID field, creating it if
// it does not already exist or overwriting it if it does.
//
// The model must declare an ID field (see the firego:"id" struct tag), and
// v's ID field must be non-empty — Firego does not auto-generate document
// IDs. Set returns an error satisfying errors.Is(err, ErrNoIDField) or
// errors.Is(err, ErrEmptyID) if either precondition isn't met.
func (r *CollectionRef[T]) Set(ctx context.Context, v T) error {
	return setDoc(ctx, r.store, r.schema, r.codec, v)
}

// Create writes v as a new document identified by v's ID field. It returns
// an error satisfying errors.Is(err, ErrAlreadyExists) if that document
// already exists.
func (r *CollectionRef[T]) Create(ctx context.Context, v T) error {
	return createDoc(ctx, r.store, r.schema, r.codec, v)
}

// Delete removes the document with id. Deleting a document that does not
// exist succeeds, matching Firestore's idempotent delete semantics.
func (r *CollectionRef[T]) Delete(ctx context.Context, id string) error {
	return deleteDoc(ctx, r.store, r.schema, id)
}

// Update changes selected fields on an existing document. FieldUpdate.Field
// names a Go struct field, which Firego resolves to its Firestore field name.
func (r *CollectionRef[T]) Update(ctx context.Context, id string, updates ...FieldUpdate) error {
	return updateDoc(ctx, r.store, r.schema, id, updates)
}

// getDoc implements Get, shared by CollectionRef and TxCollectionRef.
func getDoc[T any](ctx context.Context, store crudStore, s *schema.Schema, c codec.Codec, id string) (T, error) {
	var v T

	if id == "" {
		return v, fmt.Errorf("firego: get %s: %w", s.Collection, ErrEmptyID)
	}
	if err := validateID(id); err != nil {
		return v, fmt.Errorf("firego: get %s/%s: %w", s.Collection, id, err)
	}

	data, err := store.getDocument(ctx, s.Collection, id)
	if err != nil {
		return v, fmt.Errorf("firego: get %s/%s: %w", s.Collection, id, err)
	}
	if err := c.Decode(data, &v); err != nil {
		return v, fmt.Errorf("firego: get %s/%s: decode: %w", s.Collection, id, err)
	}
	if err := c.SetID(&v, id); err != nil {
		return v, fmt.Errorf("firego: get %s/%s: set id: %w", s.Collection, id, err)
	}
	return v, nil
}

// setDoc implements Set, shared by CollectionRef and TxCollectionRef.
func setDoc[T any](ctx context.Context, store crudStore, s *schema.Schema, c codec.Codec, v T) error {
	if s.IDField == nil {
		return fmt.Errorf("firego: %s: %w", s.Name, ErrNoIDField)
	}

	id, err := c.ID(v)
	if err != nil {
		return fmt.Errorf("firego: set %s: %w", s.Collection, err)
	}
	if id == "" {
		return fmt.Errorf("firego: %s.%s: %w", s.Name, s.IDField.Name, ErrEmptyID)
	}
	if err := validateID(id); err != nil {
		return fmt.Errorf("firego: set %s/%s: %w", s.Collection, id, err)
	}

	data, err := c.Encode(v)
	if err != nil {
		return fmt.Errorf("firego: set %s/%s: encode: %w", s.Collection, id, err)
	}
	if err := store.setDocument(ctx, s.Collection, id, data); err != nil {
		return fmt.Errorf("firego: set %s/%s: %w", s.Collection, id, err)
	}
	return nil
}

// createDoc implements Create, shared by CollectionRef and TxCollectionRef.
func createDoc[T any](ctx context.Context, store crudStore, s *schema.Schema, c codec.Codec, v T) error {
	if s.IDField == nil {
		return fmt.Errorf("firego: %s: %w", s.Name, ErrNoIDField)
	}

	id, err := c.ID(v)
	if err != nil {
		return fmt.Errorf("firego: create %s: %w", s.Collection, err)
	}
	if id == "" {
		return fmt.Errorf("firego: %s.%s: %w", s.Name, s.IDField.Name, ErrEmptyID)
	}
	if err := validateID(id); err != nil {
		return fmt.Errorf("firego: create %s/%s: %w", s.Collection, id, err)
	}

	data, err := c.Encode(v)
	if err != nil {
		return fmt.Errorf("firego: create %s/%s: encode: %w", s.Collection, id, err)
	}
	if err := store.createDocument(ctx, s.Collection, id, data); err != nil {
		return fmt.Errorf("firego: create %s/%s: %w", s.Collection, id, err)
	}
	return nil
}

// deleteDoc implements Delete, shared by CollectionRef and TxCollectionRef.
func deleteDoc(ctx context.Context, store crudStore, s *schema.Schema, id string) error {
	if id == "" {
		return fmt.Errorf("firego: delete %s: %w", s.Collection, ErrEmptyID)
	}
	if err := validateID(id); err != nil {
		return fmt.Errorf("firego: delete %s/%s: %w", s.Collection, id, err)
	}
	if err := store.deleteDocument(ctx, s.Collection, id); err != nil {
		return fmt.Errorf("firego: delete %s/%s: %w", s.Collection, id, err)
	}
	return nil
}

// updateDoc implements Update, shared by CollectionRef and TxCollectionRef.
func updateDoc(ctx context.Context, store crudStore, s *schema.Schema, id string, updates []FieldUpdate) error {
	if id == "" {
		return fmt.Errorf("firego: update %s: %w", s.Collection, ErrEmptyID)
	}
	if err := validateID(id); err != nil {
		return fmt.Errorf("firego: update %s/%s: %w", s.Collection, id, err)
	}
	if len(updates) == 0 {
		return fmt.Errorf("firego: update %s/%s: %w", s.Collection, id, ErrEmptyUpdates)
	}

	resolved := make([]documentUpdate, len(updates))
	for i, update := range updates {
		field, ok := s.FieldByName(update.Field)
		if !ok {
			return fmt.Errorf("firego: update %s/%s: field %q: %w", s.Collection, id, update.Field, ErrUnknownField)
		}
		if field.IsID {
			return fmt.Errorf("firego: update %s/%s: field %q: %w", s.Collection, id, update.Field, ErrIDFieldNotWritable)
		}

		value := update.Value
		sentinel, err := validateFirestoreSentinel(value, field.GoType)
		if err != nil {
			return fmt.Errorf("firego: update %s/%s: field %q: %w: %v", s.Collection, id, update.Field, ErrInvalidUpdateValue, err)
		}
		if !sentinel {
			value, err = codec.NormalizeValue(value, field.GoType)
			if err != nil {
				return fmt.Errorf("firego: update %s/%s: field %q: %w: %v", s.Collection, id, update.Field, ErrInvalidUpdateValue, err)
			}
		}
		resolved[i] = documentUpdate{field: field.FirestoreName, value: value}
	}

	if err := store.updateDocument(ctx, s.Collection, id, resolved); err != nil {
		return fmt.Errorf("firego: update %s/%s: %w", s.Collection, id, err)
	}
	return nil
}
