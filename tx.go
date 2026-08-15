package firego

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Tx represents an in-progress Firestore transaction, obtained from
// RunTransaction. A follow-up change adds CollectionRef[T].Tx, a
// transaction-scoped reference for reading and writing typed documents
// through it; for now Tx exposes the underlying crudStore operations that
// API will build on.
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
//
// Firestore validates a write's preconditions — Create's document-must-not-
// exist, Update's document-must-exist — against the server when the
// transaction commits, not when the Tx method that issued the write is
// called (that call only enqueues it). So an error satisfying
// errors.Is(err, ErrAlreadyExists) or errors.Is(err, ErrNotFound) caused by
// a write inside fn surfaces from RunTransaction itself, not from the
// Create or Update call that issued it.
func RunTransaction(ctx context.Context, c *Client, fn func(ctx context.Context, tx *Tx) error) error {
	if c == nil || c.firestore == nil {
		return ErrInvalidClient
	}
	err := c.firestore.RunTransaction(ctx, func(ctx context.Context, ft *firestore.Transaction) error {
		if err := fn(ctx, &Tx{client: c, tx: ft}); err != nil {
			return &fnError{err: err}
		}
		return nil
	})
	var wrapped *fnError
	if errors.As(err, &wrapped) {
		return wrapped.err
	}
	return mapCommitError(err)
}

// fnError marks an error as having come from RunTransaction's callback,
// rather than from Firestore committing the transaction, so RunTransaction
// can return it unchanged instead of running it through mapCommitError. A
// callback can itself return a gRPC AlreadyExists or NotFound error (e.g.
// from an unrelated service call); without this distinction that error
// would be misreported as a failed Create or Update.
//
// Unwrap exposes the original error to errors.As, so the underlying SDK's
// own status.FromError-based checks (its retry-on-Aborted logic) still see
// through the wrapper.
type fnError struct {
	err error
}

func (e *fnError) Error() string { return e.err.Error() }
func (e *fnError) Unwrap() error { return e.err }

// mapCommitError maps errors Firestore reports only once a transaction
// commits — as opposed to when the Tx write that caused them was issued —
// to Firego's public sentinels.
func mapCommitError(err error) error {
	switch {
	case err == nil:
		return nil
	case status.Code(err) == codes.AlreadyExists:
		return fmt.Errorf("firego: transaction commit: %w", ErrAlreadyExists)
	case isNotFound(err):
		return fmt.Errorf("firego: transaction commit: %w", ErrNotFound)
	default:
		return err
	}
}

func (t *Tx) getDocument(ctx context.Context, collection, id string) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snap, err := t.tx.Get(t.client.firestore.Collection(collection).Doc(id))
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("client: get %s/%s: %w", collection, id, err)
	}
	return snap.Data(), nil
}

func (t *Tx) setDocument(ctx context.Context, collection, id string, data map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := t.tx.Set(t.client.firestore.Collection(collection).Doc(id), data); err != nil {
		return fmt.Errorf("client: set %s/%s: %w", collection, id, err)
	}
	return nil
}

func (t *Tx) createDocument(ctx context.Context, collection, id string, data map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := t.tx.Create(t.client.firestore.Collection(collection).Doc(id), data); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return ErrAlreadyExists
		}
		return fmt.Errorf("client: create %s/%s: %w", collection, id, err)
	}
	return nil
}

func (t *Tx) deleteDocument(ctx context.Context, collection, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := t.tx.Delete(t.client.firestore.Collection(collection).Doc(id)); err != nil {
		return fmt.Errorf("client: delete %s/%s: %w", collection, id, err)
	}
	return nil
}

func (t *Tx) updateDocument(ctx context.Context, collection, id string, updates []documentUpdate) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := t.tx.Update(t.client.firestore.Collection(collection).Doc(id), toFirestoreUpdates(updates)); err != nil {
		if isNotFound(err) {
			return ErrNotFound
		}
		return fmt.Errorf("client: update %s/%s: %w", collection, id, err)
	}
	return nil
}
