package firego

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/mimu-y10/firego/codec"
	"github.com/mimu-y10/firego/schema"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Tx represents an in-progress Firestore transaction, obtained from
// RunTransaction. Use CollectionRef[T].Tx to get a transaction-scoped
// reference for reading and writing typed documents within it.
type Tx struct {
	client *Client
	tx     *firestore.Transaction
}

var _ crudStore = (*Tx)(nil)

// RunTransaction runs fn inside a Firestore transaction. If fn returns nil,
// Firestore commits every write made through tx atomically; if fn returns an
// error, or the transaction fails due to contention, Firestore retries fn or
// gives up and returns the error, per the underlying SDK's retry policy.
//
// All reads made through tx must happen before any writes, matching a
// requirement of the underlying Firestore transaction API.
func RunTransaction(ctx context.Context, c *Client, fn func(ctx context.Context, tx *Tx) error) error {
	if c == nil || c.firestore == nil {
		return ErrInvalidClient
	}
	return c.firestore.RunTransaction(ctx, func(ctx context.Context, ft *firestore.Transaction) error {
		return fn(ctx, &Tx{client: c, tx: ft})
	})
}

func (t *Tx) getDocument(_ context.Context, collection, id string) (map[string]any, error) {
	snap, err := t.tx.Get(t.client.firestore.Collection(collection).Doc(id))
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("client: get %s/%s: %w", collection, id, err)
	}
	return snap.Data(), nil
}

func (t *Tx) setDocument(_ context.Context, collection, id string, data map[string]any) error {
	if err := t.tx.Set(t.client.firestore.Collection(collection).Doc(id), data); err != nil {
		return fmt.Errorf("client: set %s/%s: %w", collection, id, err)
	}
	return nil
}

func (t *Tx) createDocument(_ context.Context, collection, id string, data map[string]any) error {
	if err := t.tx.Create(t.client.firestore.Collection(collection).Doc(id), data); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return ErrAlreadyExists
		}
		return fmt.Errorf("client: create %s/%s: %w", collection, id, err)
	}
	return nil
}

func (t *Tx) deleteDocument(_ context.Context, collection, id string) error {
	if err := t.tx.Delete(t.client.firestore.Collection(collection).Doc(id)); err != nil {
		return fmt.Errorf("client: delete %s/%s: %w", collection, id, err)
	}
	return nil
}

func (t *Tx) updateDocument(_ context.Context, collection, id string, updates []documentUpdate) error {
	if err := t.tx.Update(t.client.firestore.Collection(collection).Doc(id), toFirestoreUpdates(updates)); err != nil {
		if isNotFound(err) {
			return ErrNotFound
		}
		return fmt.Errorf("client: update %s/%s: %w", collection, id, err)
	}
	return nil
}

// TxCollectionRef is a type-safe, transaction-scoped reference to a
// Firestore collection, obtained from CollectionRef[T].Tx. Its Get, Set,
// Create, Delete, and Update methods behave exactly like CollectionRef[T]'s,
// except that reads and writes happen through tx instead of as standalone
// requests. Querying (Where/Documents) is not available within a
// transaction.
type TxCollectionRef[T any] struct {
	store  crudStore
	schema *schema.Schema
	codec  codec.Codec
}

// Tx returns a transaction-scoped reference to r's collection. Use it inside
// a RunTransaction callback in place of r itself.
func (r *CollectionRef[T]) Tx(tx *Tx) *TxCollectionRef[T] {
	return &TxCollectionRef[T]{store: tx, schema: r.schema, codec: r.codec}
}

// Get reads the document with the given ID and decodes it into a T. See
// CollectionRef[T].Get for the full contract.
func (r *TxCollectionRef[T]) Get(ctx context.Context, id string) (T, error) {
	return getDoc[T](ctx, r.store, r.schema, r.codec, id)
}

// Set writes v to the document identified by v's ID field. See
// CollectionRef[T].Set for the full contract.
func (r *TxCollectionRef[T]) Set(ctx context.Context, v T) error {
	return setDoc(ctx, r.store, r.schema, r.codec, v)
}

// Create writes v as a new document identified by v's ID field. See
// CollectionRef[T].Create for the full contract.
func (r *TxCollectionRef[T]) Create(ctx context.Context, v T) error {
	return createDoc(ctx, r.store, r.schema, r.codec, v)
}

// Delete removes the document with id. See CollectionRef[T].Delete for the
// full contract.
func (r *TxCollectionRef[T]) Delete(ctx context.Context, id string) error {
	return deleteDoc(ctx, r.store, r.schema, id)
}

// Update changes selected fields on an existing document. See
// CollectionRef[T].Update for the full contract.
func (r *TxCollectionRef[T]) Update(ctx context.Context, id string, updates ...FieldUpdate) error {
	return updateDoc(ctx, r.store, r.schema, id, updates)
}
