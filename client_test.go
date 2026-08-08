package firego

import (
	"errors"
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/mimu-y10/firego/schema"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNewClientFromFirestore(t *testing.T) {
	fs := &firestore.Client{}
	client, err := NewClientFromFirestore(fs)
	if err != nil {
		t.Fatalf("NewClientFromFirestore() error = %v", err)
	}

	if client.firestore != fs {
		t.Error("NewClientFromFirestore() did not preserve the underlying Firestore client")
	}
	if client.registry == nil {
		t.Error("NewClientFromFirestore() did not initialize the schema registry")
	}
}

func TestNewClientFromFirestoreRejectsNil(t *testing.T) {
	client, err := NewClientFromFirestore(nil)
	if client != nil {
		t.Errorf("NewClientFromFirestore(nil) client = %v, want nil", client)
	}
	if !errors.Is(err, ErrNilFirestoreClient) {
		t.Fatalf("NewClientFromFirestore(nil) error = %v, want errors.Is(err, ErrNilFirestoreClient)", err)
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "not found status", err: status.Error(codes.NotFound, `"users/missing" not found`), want: true},
		{name: "different status code", err: status.Error(codes.PermissionDenied, "denied"), want: false},
		{name: "plain error", err: errors.New("boom"), want: false},
		{name: "nil error", err: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNotFound(tt.err); got != tt.want {
				t.Errorf("isNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestCloneSchemaDetachesFromOriginal(t *testing.T) {
	original := &schema.Schema{
		Name:       "Model",
		Collection: "models",
		Fields: []schema.Field{
			{Name: "ID", FirestoreName: "-", StructIndex: []int{0}, IsID: true},
			{Name: "Value", FirestoreName: "value", StructIndex: []int{1}},
		},
	}
	original.IDField = &original.Fields[0]

	clone := cloneSchema(original)
	clone.Fields[0].Name = "mutated"
	clone.Fields[1].StructIndex[0] = 999
	clone.IDField.FirestoreName = "mutated"

	if original.Fields[0].Name != "ID" {
		t.Errorf("mutating clone.Fields[0].Name changed the original: %q", original.Fields[0].Name)
	}
	if original.Fields[1].StructIndex[0] != 1 {
		t.Errorf("mutating clone.Fields[1].StructIndex changed the original: %v", original.Fields[1].StructIndex)
	}
	if original.IDField.FirestoreName != "-" {
		t.Errorf("mutating clone.IDField changed the original: %q", original.IDField.FirestoreName)
	}
}

func TestCloneSchemaIDFieldPointsIntoTheClone(t *testing.T) {
	original := &schema.Schema{Fields: []schema.Field{{Name: "ID", IsID: true, StructIndex: []int{0}}}}
	original.IDField = &original.Fields[0]
	clone := cloneSchema(original)
	if clone.IDField != &clone.Fields[0] {
		t.Error("clone.IDField does not point into clone.Fields")
	}
}

func TestCloneSchemaWithoutIDField(t *testing.T) {
	original := &schema.Schema{Fields: []schema.Field{{Name: "Value", StructIndex: []int{0}}}}
	if clone := cloneSchema(original); clone.IDField != nil {
		t.Errorf("IDField = %v, want nil", clone.IDField)
	}
}
