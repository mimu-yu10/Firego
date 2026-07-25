package codec

import (
	"testing"
	"time"

	"github.com/mimu-y10/firego/internal/metadata"
)

type testModel struct {
	ID        string `firego:"id" firestore:"-"`
	Name      string `firestore:"name"`
	Age       int
	CreatedAt time.Time `firestore:"created_at"`
}

func mustParse(t *testing.T) *schemaCodec {
	t.Helper()
	s, err := metadata.Parse[testModel]("models")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return &schemaCodec{schema: s}
}

func TestEncode(t *testing.T) {
	c := mustParse(t)
	now := time.Now()

	got, err := c.Encode(testModel{ID: "abc", Name: "Alice", Age: 30, CreatedAt: now})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	if _, ok := got["ID"]; ok {
		t.Errorf("Encode() included ID field in document data: %v", got)
	}
	if got["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", got["name"])
	}
	if got["Age"] != 30 {
		t.Errorf("Age = %v, want 30", got["Age"])
	}
	if got["created_at"] != now {
		t.Errorf("created_at = %v, want %v", got["created_at"], now)
	}
	if len(got) != 3 {
		t.Errorf("len(data) = %d, want 3; data = %v", len(got), got)
	}
}

func TestEncodePointer(t *testing.T) {
	c := mustParse(t)

	got, err := c.Encode(&testModel{Name: "Bob"})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if got["name"] != "Bob" {
		t.Errorf("name = %v, want Bob", got["name"])
	}
}

func TestEncodeRejectsNilPointer(t *testing.T) {
	c := mustParse(t)

	var v *testModel
	if _, err := c.Encode(v); err == nil {
		t.Fatal("Encode() error = nil, want error")
	}
}

func TestEncodeTypeMismatch(t *testing.T) {
	c := mustParse(t)

	type other struct{ X int }
	if _, err := c.Encode(other{}); err == nil {
		t.Fatal("Encode() error = nil, want error")
	}
}

func TestDecode(t *testing.T) {
	c := mustParse(t)
	now := time.Now()

	data := map[string]any{
		"name":       "Carol",
		"Age":        int64(25), // Firestore returns whole numbers as int64.
		"created_at": now,
	}

	var got testModel
	if err := c.Decode(data, &got); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if got.Name != "Carol" {
		t.Errorf("Name = %q, want Carol", got.Name)
	}
	if got.Age != 25 {
		t.Errorf("Age = %d, want 25", got.Age)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, now)
	}
	if got.ID != "" {
		t.Errorf("ID = %q, want empty (codec must not set ID)", got.ID)
	}
}

func TestDecodeMissingFieldsLeftZero(t *testing.T) {
	c := mustParse(t)

	var got testModel
	if err := c.Decode(map[string]any{"name": "Dana"}, &got); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got.Age != 0 {
		t.Errorf("Age = %d, want 0", got.Age)
	}
}

func TestDecodeRejectsNonPointer(t *testing.T) {
	c := mustParse(t)

	if err := c.Decode(map[string]any{}, testModel{}); err == nil {
		t.Fatal("Decode() error = nil, want error")
	}
}

func TestDecodeRejectsNilPointer(t *testing.T) {
	c := mustParse(t)

	var dst *testModel
	if err := c.Decode(map[string]any{}, dst); err == nil {
		t.Fatal("Decode() error = nil, want error")
	}
}

func TestDecodeTypeMismatch(t *testing.T) {
	c := mustParse(t)

	type other struct{ X int }
	var dst other
	if err := c.Decode(map[string]any{}, &dst); err == nil {
		t.Fatal("Decode() error = nil, want error")
	}
}

func TestDecodeIncompatibleValue(t *testing.T) {
	c := mustParse(t)

	var got testModel
	if err := c.Decode(map[string]any{"Age": "not a number"}, &got); err == nil {
		t.Fatal("Decode() error = nil, want error")
	}
}
