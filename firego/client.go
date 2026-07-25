package firego

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/mimu-y10/firego/client"
	"google.golang.org/api/option"
)

// Client is the Firego client, backed by a Google Cloud Firestore client.
// It is a type alias for client.Client so callers never need to import the
// client package themselves.
type Client = client.Client

// ErrNotFound is returned when a requested document does not exist.
var ErrNotFound = client.ErrNotFound

// NewClient creates a Firego client for the given Google Cloud project.
func NewClient(ctx context.Context, projectID string, opts ...option.ClientOption) (*Client, error) {
	fs, err := firestore.NewClient(ctx, projectID, opts...)
	if err != nil {
		return nil, err
	}
	return client.New(fs), nil
}
