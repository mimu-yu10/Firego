package firego

import (
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/mimu-y10/firego/codec"
	"github.com/mimu-y10/firego/internal/metadata"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newTestTxRef builds a TxCollectionRef against a fake crudStore, mirroring
// newTestRef. TxCollectionRef and CollectionRef share their orchestration
// logic (getDoc/setDoc/etc.), so these tests focus on wiring TxCollectionRef
// to that shared logic rather than re-covering it in full.
func newTestTxRef[T any](t *testing.T, store crudStore, collection string) *TxCollectionRef[T] {
	t.Helper()
	s, err := metadata.Parse[T](collection)
	if err != nil {
		t.Fatalf("metadata.Parse() error = %v", err)
	}
	return &TxCollectionRef[T]{store: store, schema: s, codec: codec.New(s)}
}

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
// the server when the transaction commits, not when the TxCollectionRef[T]
// method that issued the write is called — so a bare gRPC status from the
// commit, rather than an error already wrapping ErrAlreadyExists/ErrNotFound,
// is exactly what RunTransaction receives in practice.
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

func TestTxCollectionRefGetDecodesAndInjectsID(t *testing.T) {
	store := newFakeStore()
	store.docs["users/abc"] = map[string]any{"name": "Alice", "Age": int64(30)}
	ref := newTestTxRef[testUser](t, store, "users")

	got, err := ref.Get(context.Background(), "abc")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	want := testUser{ID: "abc", Name: "Alice", Age: 30}
	if got != want {
		t.Errorf("Get() = %+v, want %+v", got, want)
	}
}

func TestTxCollectionRefSetThenGetRoundTrip(t *testing.T) {
	store := newFakeStore()
	ref := newTestTxRef[testUser](t, store, "users")

	want := testUser{ID: "abc", Name: "Alice", Age: 30}
	if err := ref.Set(context.Background(), want); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	got, err := ref.Get(context.Background(), "abc")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestTxCollectionRefCreateRejectsExisting(t *testing.T) {
	store := newFakeStore()
	ref := newTestTxRef[testUser](t, store, "users")

	v := testUser{ID: "abc", Name: "Alice", Age: 30}
	if err := ref.Create(context.Background(), v); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	if err := ref.Create(context.Background(), v); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("second Create() error = %v, want errors.Is(err, ErrAlreadyExists)", err)
	}
}

func TestTxCollectionRefDeleteIsIdempotent(t *testing.T) {
	store := newFakeStore()
	ref := newTestTxRef[testUser](t, store, "users")

	if err := ref.Delete(context.Background(), "missing"); err != nil {
		t.Fatalf("Delete() error = %v, want nil (idempotent)", err)
	}
}

func TestTxCollectionRefUpdateResolvesFieldName(t *testing.T) {
	store := newFakeStore()
	store.docs["users/abc"] = map[string]any{"name": "Alice", "Age": 30}
	ref := newTestTxRef[testUser](t, store, "users")

	if err := ref.Update(context.Background(), "abc", FieldUpdate{Field: "Age", Value: 31}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	got, err := ref.Get(context.Background(), "abc")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Age != 31 {
		t.Errorf("Age = %d, want 31", got.Age)
	}
}

func TestCollectionRefTxReusesSchemaAndCodec(t *testing.T) {
	client, err := NewClientFromFirestore(&firestore.Client{})
	if err != nil {
		t.Fatalf("NewClientFromFirestore() error = %v", err)
	}
	ref, err := Collection[testUser](client, "users")
	if err != nil {
		t.Fatalf("Collection() error = %v", err)
	}

	txRef, err := ref.Tx(&Tx{client: client})
	if err != nil {
		t.Fatalf("Tx() error = %v", err)
	}
	if txRef.schema != ref.schema {
		t.Error("Tx() did not carry over the CollectionRef's schema")
	}
}

func TestCollectionRefTxRejectsMismatchedClient(t *testing.T) {
	clientA, err := NewClientFromFirestore(&firestore.Client{})
	if err != nil {
		t.Fatalf("NewClientFromFirestore() error = %v", err)
	}
	clientB, err := NewClientFromFirestore(&firestore.Client{})
	if err != nil {
		t.Fatalf("NewClientFromFirestore() error = %v", err)
	}
	ref, err := Collection[testUser](clientA, "users")
	if err != nil {
		t.Fatalf("Collection() error = %v", err)
	}

	// tx was opened on a different Client than ref was created from (e.g. a
	// different named database); Tx must reject it rather than building
	// document references against the wrong one.
	if _, err := ref.Tx(&Tx{client: clientB}); !errors.Is(err, ErrClientMismatch) {
		t.Fatalf("Tx() error = %v, want errors.Is(err, ErrClientMismatch)", err)
	}
}

func TestCollectionRefTxRejectsNilTx(t *testing.T) {
	client, err := NewClientFromFirestore(&firestore.Client{})
	if err != nil {
		t.Fatalf("NewClientFromFirestore() error = %v", err)
	}
	ref, err := Collection[testUser](client, "users")
	if err != nil {
		t.Fatalf("Collection() error = %v", err)
	}

	if _, err := ref.Tx(nil); !errors.Is(err, ErrClientMismatch) {
		t.Fatalf("Tx(nil) error = %v, want errors.Is(err, ErrClientMismatch)", err)
	}
}
