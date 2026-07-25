package metadata

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mimu-y10/firego/schema"
)

type testUser struct {
	ID       string `firego:"id" firestore:"-"`
	Name     string `firestore:"name,omitempty"`
	Age      int
	Ignored  string `firestore:"-"`
	internal string //nolint:unused // present so the parser can be tested against an unexported field
}

// testBase is a reusable embedded struct that models the common Base pattern.
type testBase struct {
	ID  string `firego:"id" firestore:"-"`
	Rev string `firestore:"_rev"`
}

type testEmbedded struct {
	testBase
	Name string `firestore:"name"`
}

// testNamedEmbed has an embedded struct WITH an explicit firestore tag name,
// which means it must NOT be promoted — it is treated as a single nested field.
// The embedded type must be exported so the tag takes effect (unexported
// anonymous fields are always promoted regardless of tags).
type TestNamedInner struct {
	Foo string `firestore:"foo"`
}
type testNamedEmbed struct {
	TestNamedInner `firestore:"inner"`
	Bar            string `firestore:"bar"`
}

func TestParse(t *testing.T) {
	got, err := Parse[*testUser]("users")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if got.Name != "testUser" {
		t.Errorf("Name = %q, want %q", got.Name, "testUser")
	}
	if got.Collection != "users" {
		t.Errorf("Collection = %q, want %q", got.Collection, "users")
	}
	if got.GoType != reflect.TypeFor[testUser]() {
		t.Errorf("GoType = %v, want %v", got.GoType, reflect.TypeFor[testUser]())
	}
	if len(got.Fields) != 3 {
		t.Fatalf("len(Fields) = %d, want 3", len(got.Fields))
	}
	if got.IDField == nil || got.IDField.Name != "ID" {
		t.Fatalf("IDField = %#v, want ID field", got.IDField)
	}
	if got.Fields[1].FirestoreName != "name" {
		t.Errorf("Name FirestoreName = %q, want %q", got.Fields[1].FirestoreName, "name")
	}
	if got.Fields[2].FirestoreName != "Age" {
		t.Errorf("Age FirestoreName = %q, want %q", got.Fields[2].FirestoreName, "Age")
	}
}

func TestParseEmbedded(t *testing.T) {
	got, err := Parse[testEmbedded]("users")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// ID + Rev (from testBase) + Name = 3 fields. The embedding struct itself
	// must not appear as a field.
	if len(got.Fields) != 3 {
		t.Fatalf("len(Fields) = %d, want 3; fields = %v", len(got.Fields), got.Fields)
	}

	if got.IDField == nil || got.IDField.Name != "ID" {
		t.Fatalf("IDField = %#v, want ID field", got.IDField)
	}

	// StructIndex for a promoted field must include the embedding position.
	// testEmbedded.testBase is at index 0; ID is index 0 within testBase → [0,0].
	wantIDIndex := []int{0, 0}
	if !reflect.DeepEqual(got.IDField.StructIndex, wantIDIndex) {
		t.Errorf("IDField.StructIndex = %v, want %v", got.IDField.StructIndex, wantIDIndex)
	}

	var revField, nameField *schema.Field
	for i := range got.Fields {
		switch got.Fields[i].Name {
		case "Rev":
			revField = &got.Fields[i]
		case "Name":
			nameField = &got.Fields[i]
		}
	}
	if revField == nil || revField.FirestoreName != "_rev" {
		t.Errorf("Rev field = %#v, want FirestoreName _rev", revField)
	}
	// Rev is testBase[1] → [0, 1].
	if !reflect.DeepEqual(revField.StructIndex, []int{0, 1}) {
		t.Errorf("Rev.StructIndex = %v, want [0 1]", revField.StructIndex)
	}
	if nameField == nil || nameField.FirestoreName != "name" {
		t.Errorf("Name field = %#v, want FirestoreName name", nameField)
	}
	if !reflect.DeepEqual(nameField.StructIndex, []int{1}) {
		t.Errorf("Name.StructIndex = %v, want [1]", nameField.StructIndex)
	}
}

func TestParseNamedEmbedNotPromoted(t *testing.T) {
	got, err := Parse[testNamedEmbed]("items")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// testNamedInner carries firestore:"inner", so it must be a single field,
	// not promoted. Fields: inner, bar.
	if len(got.Fields) != 2 {
		t.Fatalf("len(Fields) = %d, want 2; fields = %v", len(got.Fields), got.Fields)
	}
	if got.Fields[0].FirestoreName != "inner" {
		t.Errorf("Fields[0].FirestoreName = %q, want %q", got.Fields[0].FirestoreName, "inner")
	}
	if got.Fields[1].FirestoreName != "bar" {
		t.Errorf("Fields[1].FirestoreName = %q, want %q", got.Fields[1].FirestoreName, "bar")
	}
}

func TestParseRejectsInvalidModels(t *testing.T) {
	tests := []struct {
		name    string
		parse   func() error
		wantErr string
	}{
		{
			name: "empty collection",
			parse: func() error {
				_, err := Parse[testUser]("")
				return err
			},
			wantErr: "collection must not be empty",
		},
		{
			name: "non-struct model",
			parse: func() error {
				_, err := Parse[string]("values")
				return err
			},
			wantErr: "model must be a struct",
		},
		{
			name: "multiple ID fields",
			parse: func() error {
				type model struct {
					First  string `firego:"id"`
					Second string `firego:"id"`
				}
				_, err := Parse[model]("models")
				return err
			},
			wantErr: "multiple ID fields",
		},
		{
			name: "multiple ID fields across embedding",
			parse: func() error {
				// Base must be exported so the embedded field is promoted.
				type Base struct {
					BaseID string `firego:"id"`
				}
				type model struct {
					Base
					OwnID string `firego:"id"`
				}
				_, err := Parse[model]("models")
				return err
			},
			wantErr: "multiple ID fields",
		},
		{
			name: "non-string ID",
			parse: func() error {
				type model struct {
					ID int `firego:"id"`
				}
				_, err := Parse[model]("models")
				return err
			},
			wantErr: "must be a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.parse()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}
