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

// noIDModel has no firego:"id" field, exercising the no-op path of ID and
// SetID.
type noIDModel struct {
	Name string `firestore:"name"`
}

func mustParseNoID(t *testing.T) *schemaCodec {
	t.Helper()
	s, err := metadata.Parse[noIDModel]("items")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return &schemaCodec{schema: s}
}

func TestID(t *testing.T) {
	c := mustParse(t)

	got, err := c.ID(testModel{ID: "abc"})
	if err != nil {
		t.Fatalf("ID() error = %v", err)
	}
	if got != "abc" {
		t.Errorf("ID() = %q, want %q", got, "abc")
	}
}

func TestIDPointer(t *testing.T) {
	c := mustParse(t)

	got, err := c.ID(&testModel{ID: "abc"})
	if err != nil {
		t.Fatalf("ID() error = %v", err)
	}
	if got != "abc" {
		t.Errorf("ID() = %q, want %q", got, "abc")
	}
}

func TestIDNoIDField(t *testing.T) {
	c := mustParseNoID(t)

	got, err := c.ID(noIDModel{Name: "x"})
	if err != nil {
		t.Fatalf("ID() error = %v", err)
	}
	if got != "" {
		t.Errorf("ID() = %q, want empty", got)
	}
}

func TestIDRejectsNilPointer(t *testing.T) {
	c := mustParse(t)

	var v *testModel
	if _, err := c.ID(v); err == nil {
		t.Fatal("ID() error = nil, want error")
	}
}

func TestIDTypeMismatch(t *testing.T) {
	c := mustParse(t)

	type other struct{ X int }
	if _, err := c.ID(other{}); err == nil {
		t.Fatal("ID() error = nil, want error")
	}
}

func TestSetID(t *testing.T) {
	c := mustParse(t)

	var got testModel
	if err := c.SetID(&got, "abc"); err != nil {
		t.Fatalf("SetID() error = %v", err)
	}
	if got.ID != "abc" {
		t.Errorf("ID = %q, want %q", got.ID, "abc")
	}
}

func TestSetIDNoIDField(t *testing.T) {
	c := mustParseNoID(t)

	var got noIDModel
	if err := c.SetID(&got, "abc"); err != nil {
		t.Fatalf("SetID() error = %v", err)
	}
}

func TestSetIDRejectsNonPointer(t *testing.T) {
	c := mustParse(t)

	if err := c.SetID(testModel{}, "abc"); err == nil {
		t.Fatal("SetID() error = nil, want error")
	}
}

func TestSetIDRejectsNilPointer(t *testing.T) {
	c := mustParse(t)

	var dst *testModel
	if err := c.SetID(dst, "abc"); err == nil {
		t.Fatal("SetID() error = nil, want error")
	}
}

func TestSetIDTypeMismatch(t *testing.T) {
	c := mustParse(t)

	type other struct{ X int }
	var dst other
	if err := c.SetID(&dst, "abc"); err == nil {
		t.Fatal("SetID() error = nil, want error")
	}
}

// PtrEmbedBase and ptrEmbedModel exercise promotion through a nil embedded
// *pointer* struct (as opposed to testBase/testEmbedded's embedding by
// value), which the metadata parser explicitly supports.
type PtrEmbedBase struct {
	ID  string `firego:"id" firestore:"-"`
	Rev string `firestore:"rev"`
}

type ptrEmbedModel struct {
	*PtrEmbedBase
	Name string `firestore:"name"`
}

func mustParsePtrEmbed(t *testing.T) *schemaCodec {
	t.Helper()
	s, err := metadata.Parse[ptrEmbedModel]("items")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return &schemaCodec{schema: s}
}

func TestSetIDAllocatesNilEmbeddedPointer(t *testing.T) {
	c := mustParsePtrEmbed(t)

	var got ptrEmbedModel // *PtrEmbedBase starts nil
	if err := c.SetID(&got, "abc"); err != nil {
		t.Fatalf("SetID() error = %v", err)
	}
	if got.PtrEmbedBase == nil {
		t.Fatal("SetID() left the embedded pointer nil")
	}
	if got.ID != "abc" {
		t.Errorf("ID = %q, want %q", got.ID, "abc")
	}
}

func TestDecodeAllocatesNilEmbeddedPointer(t *testing.T) {
	c := mustParsePtrEmbed(t)

	var got ptrEmbedModel // *PtrEmbedBase starts nil
	if err := c.Decode(map[string]any{"rev": "r1", "name": "x"}, &got); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got.PtrEmbedBase == nil {
		t.Fatal("Decode() left the embedded pointer nil")
	}
	if got.Rev != "r1" {
		t.Errorf("Rev = %q, want %q", got.Rev, "r1")
	}
	if got.Name != "x" {
		t.Errorf("Name = %q, want %q", got.Name, "x")
	}
}

func TestEncodeHandlesNilEmbeddedPointer(t *testing.T) {
	c := mustParsePtrEmbed(t)

	got, err := c.Encode(ptrEmbedModel{Name: "x"}) // *PtrEmbedBase left nil
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if got["rev"] != "" {
		t.Errorf("rev = %v, want zero value", got["rev"])
	}
	if got["name"] != "x" {
		t.Errorf("name = %v, want x", got["name"])
	}
}

func TestIDHandlesNilEmbeddedPointer(t *testing.T) {
	c := mustParsePtrEmbed(t)

	got, err := c.ID(ptrEmbedModel{Name: "x"}) // *PtrEmbedBase left nil
	if err != nil {
		t.Fatalf("ID() error = %v", err)
	}
	if got != "" {
		t.Errorf("ID() = %q, want empty", got)
	}
}

func TestIDRejectsUntypedNil(t *testing.T) {
	c := mustParse(t)

	if _, err := c.ID(nil); err == nil {
		t.Fatal("ID() error = nil, want error")
	}
}

func TestEncodeRejectsUntypedNil(t *testing.T) {
	c := mustParse(t)

	if _, err := c.Encode(nil); err == nil {
		t.Fatal("Encode() error = nil, want error")
	}
}
