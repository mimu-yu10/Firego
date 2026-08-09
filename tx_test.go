package firego

import (
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/mimu-y10/firego/codec"
	"github.com/mimu-y10/firego/internal/metadata"
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

	txRef := ref.Tx(&Tx{})
	if txRef.schema != ref.schema {
		t.Error("Tx() did not carry over the CollectionRef's schema")
	}
}
