// Package client provides the Firestore client used by Firego.
package client

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/mimu-y10/firego/internal/metadata"
	"github.com/mimu-y10/firego/schema"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrNotFound is returned by GetDocument when the requested document does
// not exist.
var ErrNotFound = errors.New("client: document not found")

// Client wraps the Google Cloud Firestore client used by Firego.
type Client struct {
	firestore *firestore.Client
	registry  *metadata.Registry
}

// New creates a Firego client backed by client.
func New(client *firestore.Client) *Client {
	return &Client{
		firestore: client,
		registry:  metadata.NewRegistry(),
	}
}

// Schema returns the schema for model T and collection, building and
// caching it on the first call for that (T, collection) pair via c's
// registry. Each call returns a copy detached from the cached original, so
// callers can never mutate what other callers (or a later Schema call for
// the same T and collection) observe.
func Schema[T any](c *Client, collection string) (*schema.Schema, error) {
	s, err := metadata.Get[T](c.registry, collection)
	if err != nil {
		return nil, err
	}
	return cloneSchema(s), nil
}

// cloneSchema returns a deep copy of s: a new Schema, a new Fields slice
// with its own backing array, and — for every Field, including a fresh
// IDField pointer into the copied slice — a new StructIndex slice. Nothing
// in the result aliases s.
func cloneSchema(s *schema.Schema) *schema.Schema {
	clone := *s
	clone.Fields = make([]schema.Field, len(s.Fields))
	clone.IDField = nil
	for i, f := range s.Fields {
		f.StructIndex = append([]int(nil), f.StructIndex...)
		clone.Fields[i] = f
		if f.IsID {
			clone.IDField = &clone.Fields[i]
		}
	}
	return &clone
}

// GetDocument reads the document at collection/id and returns its data.
// It returns ErrNotFound if the document does not exist.
func (c *Client) GetDocument(ctx context.Context, collection, id string) (map[string]any, error) {
	snap, err := c.firestore.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("client: get %s/%s: %w", collection, id, err)
	}
	return snap.Data(), nil
}

// SetDocument writes data to the document at collection/id, creating it if
// it does not already exist or overwriting it if it does.
func (c *Client) SetDocument(ctx context.Context, collection, id string, data map[string]any) error {
	if _, err := c.firestore.Collection(collection).Doc(id).Set(ctx, data); err != nil {
		return fmt.Errorf("client: set %s/%s: %w", collection, id, err)
	}
	return nil
}

// Close releases resources (such as open gRPC connections) held by the
// underlying Firestore client. Call it when the Client is no longer needed.
func (c *Client) Close() error {
	return c.firestore.Close()
}

// isNotFound reports whether err is the gRPC status Firestore's SDK returns
// when a requested document does not exist.
func isNotFound(err error) bool {
	return status.Code(err) == codes.NotFound
}
