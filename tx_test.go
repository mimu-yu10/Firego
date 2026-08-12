package firego

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRunTransactionRejectsInvalidClient(t *testing.T) {
	err := RunTransaction(context.Background(), nil, func(context.Context, *Tx) error {
		t.Fatal("fn should not run when the client is invalid")
		return nil
	})
	if !errors.Is(err, ErrInvalidClient) {
		t.Fatalf("RunTransaction() error = %v, want errors.Is(err, ErrInvalidClient)", err)
	}
}

func TestRunTransactionRejectsZeroValueClient(t *testing.T) {
	err := RunTransaction(context.Background(), &Client{}, func(context.Context, *Tx) error {
		t.Fatal("fn should not run when the client is invalid")
		return nil
	})
	if !errors.Is(err, ErrInvalidClient) {
		t.Fatalf("RunTransaction() error = %v, want errors.Is(err, ErrInvalidClient)", err)
	}
}

// TestMapCommitError guards RunTransaction's translation of commit-time
// Firestore errors to Firego's sentinels. Create's document-must-not-exist
// and Update's document-must-exist preconditions are only validated against
// the server when the transaction commits, not when the Tx method that
// issued the write is called — so a bare gRPC status from the commit,
// rather than an error already wrapping ErrAlreadyExists/ErrNotFound, is
// exactly what RunTransaction receives in practice.
// TestFnErrorPreservesStatusAndUnwraps guards the mechanism RunTransaction
// relies on to tell a callback-returned error apart from a commit-time one:
// fnError must still expose the original gRPC status through errors.As (so
// mapCommitError doesn't misclassify a callback's own AlreadyExists/NotFound
// as a failed Create/Update), and status.Code must still see through it (so
// the underlying SDK's retry-on-Aborted logic isn't disturbed by wrapping).
func TestFnErrorPreservesStatusAndUnwraps(t *testing.T) {
	original := status.Error(codes.AlreadyExists, "boom")
	wrapped := &fnError{err: original}

	if got := wrapped.Unwrap(); got != original {
		t.Errorf("Unwrap() = %v, want %v", got, original)
	}

	var target *fnError
	if !errors.As(error(wrapped), &target) {
		t.Fatal("errors.As(wrapped, &fnError{}) = false, want true")
	}
	if target.err != original {
		t.Errorf("target.err = %v, want %v", target.err, original)
	}

	if code := status.Code(wrapped); code != codes.AlreadyExists {
		t.Errorf("status.Code(wrapped) = %v, want %v", code, codes.AlreadyExists)
	}
}

func TestMapCommitError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "nil", err: nil, want: nil},
		{name: "already exists", err: status.Error(codes.AlreadyExists, "boom"), want: ErrAlreadyExists},
		{name: "not found", err: status.Error(codes.NotFound, "boom"), want: ErrNotFound},
		{name: "unrelated", err: errors.New("boom"), want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapCommitError(tt.err)
			if tt.want == nil {
				if got != tt.err {
					t.Errorf("mapCommitError(%v) = %v, want unchanged", tt.err, got)
				}
				return
			}
			if !errors.Is(got, tt.want) {
				t.Errorf("mapCommitError(%v) = %v, want errors.Is(err, %v)", tt.err, got, tt.want)
			}
		})
	}
}

func TestTxOperationsRejectCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tx := &Tx{}

	if _, err := tx.getDocument(ctx, "users", "abc"); !errors.Is(err, context.Canceled) {
		t.Errorf("getDocument() error = %v, want errors.Is(err, context.Canceled)", err)
	}
	if err := tx.setDocument(ctx, "users", "abc", nil); !errors.Is(err, context.Canceled) {
		t.Errorf("setDocument() error = %v, want errors.Is(err, context.Canceled)", err)
	}
	if err := tx.createDocument(ctx, "users", "abc", nil); !errors.Is(err, context.Canceled) {
		t.Errorf("createDocument() error = %v, want errors.Is(err, context.Canceled)", err)
	}
	if err := tx.deleteDocument(ctx, "users", "abc"); !errors.Is(err, context.Canceled) {
		t.Errorf("deleteDocument() error = %v, want errors.Is(err, context.Canceled)", err)
	}
	if err := tx.updateDocument(ctx, "users", "abc", nil); !errors.Is(err, context.Canceled) {
		t.Errorf("updateDocument() error = %v, want errors.Is(err, context.Canceled)", err)
	}
}
