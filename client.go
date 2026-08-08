package firego

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/mimu-y10/firego/internal/metadata"
	"github.com/mimu-y10/firego/query"
	"github.com/mimu-y10/firego/schema"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrNotFound is returned when a requested document does not exist.
var ErrNotFound = errors.New("client: document not found")

// Client wraps the Google Cloud Firestore client used by Firego.
type Client struct {
	firestore *firestore.Client
	registry  *metadata.Registry
}

// NewClient creates a Firego client for the given Google Cloud project.
func NewClient(ctx context.Context, projectID string, opts ...option.ClientOption) (*Client, error) {
	fs, err := firestore.NewClient(ctx, projectID, opts...)
	if err != nil {
		return nil, err
	}
	return NewClientFromFirestore(fs), nil
}

// NewClientFromFirestore creates a Firego client backed by an existing
// Firestore client. Use it when the underlying client needs configuration
// that NewClient does not expose, such as a named database.
func NewClientFromFirestore(client *firestore.Client) *Client {
	return &Client{
		firestore: client,
		registry:  metadata.NewRegistry(),
	}
}

// schemaFor returns the schema for model T and collection, building and
// caching it on the first call for that pair. The returned schema is detached
// from the cache so callers cannot mutate later results.
func schemaFor[T any](c *Client, collection string) (*schema.Schema, error) {
	s, err := metadata.Get[T](c.registry, collection)
	if err != nil {
		return nil, err
	}
	return cloneSchema(s), nil
}

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

// getDocument reads the document at collection/id and returns its data.
// It returns ErrNotFound if the document does not exist.
func (c *Client) getDocument(ctx context.Context, collection, id string) (map[string]any, error) {
	snap, err := c.firestore.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("client: get %s/%s: %w", collection, id, err)
	}
	return snap.Data(), nil
}

// setDocument writes data to the document at collection/id, creating it if
// necessary or overwriting it if it already exists.
func (c *Client) setDocument(ctx context.Context, collection, id string, data map[string]any) error {
	if _, err := c.firestore.Collection(collection).Doc(id).Set(ctx, data); err != nil {
		return fmt.Errorf("client: set %s/%s: %w", collection, id, err)
	}
	return nil
}

// document is one result of a queryDocuments call: a document ID and its data.
type document struct {
	ID   string
	Data map[string]any
}

// queryDocuments returns every document in collection that matches all filters.
func (c *Client) queryDocuments(ctx context.Context, collection string, filters []query.Filter) ([]document, error) {
	q := c.firestore.Collection(collection).Query
	for _, f := range filters {
		q = q.WherePath(firestore.FieldPath{f.Field}, f.Op, f.Value)
	}

	iter := q.Documents(ctx)
	defer iter.Stop()

	var docs []document
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("client: query %s: %w", collection, err)
		}
		docs = append(docs, document{ID: snap.Ref.ID, Data: snap.Data()})
	}
	return docs, nil
}

// Close releases resources held by the underlying Firestore client.
func (c *Client) Close() error {
	return c.firestore.Close()
}

func isNotFound(err error) bool {
	return status.Code(err) == codes.NotFound
}
