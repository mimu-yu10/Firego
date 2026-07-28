// Package schema describes the mapping between Go types and Firestore
// collections, documents, and fields.
package schema

import "reflect"

// Schema contains the metadata needed to map one Go struct to a Firestore
// collection and its documents.
type Schema struct {
	// Name is the Go model name, such as "User".
	Name string

	// Collection is the Firestore collection path, such as "users".
	Collection string

	// GoType is the runtime type of the model struct. It is used to validate
	// model values and create new values when decoding documents.
	GoType reflect.Type

	// Fields contains metadata for every mapped field in the model.
	Fields []Field

	// IDField points to the field that receives the Firestore document ID.
	// It is nil when the model does not declare an ID field.
	IDField *Field
}

// Field contains the metadata needed to map one Go struct field to a
// Firestore document field.
type Field struct {
	// Name is the Go struct field name, such as "CreatedAt".
	Name string

	// FirestoreName is the name stored in Firestore, such as "created_at".
	// An ID field normally uses "-" because a document ID is not document data.
	FirestoreName string

	// GoType is the runtime type of the field, such as string or time.Time.
	GoType reflect.Type

	// StructIndex identifies the field's position inside the Go struct.
	// For example, the first field has []int{0}. This is a Go reflection path,
	// not a Firestore index.
	StructIndex []int

	// IsID reports whether this field receives the Firestore document ID.
	IsID bool
}

// FieldByName returns the field named name — the Go struct field name, not
// its FirestoreName — and reports whether it was found.
func (s *Schema) FieldByName(name string) (*Field, bool) {
	for i := range s.Fields {
		if s.Fields[i].Name == name {
			return &s.Fields[i], true
		}
	}
	return nil, false
}
