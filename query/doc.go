// Package query provides types for building Firestore queries.
package query

// Operator is a Firestore query comparison operator.
type Operator string

const (
	Equal              Operator = "=="
	NotEqual           Operator = "!="
	LessThan           Operator = "<"
	LessThanOrEqual    Operator = "<="
	GreaterThan        Operator = ">"
	GreaterThanOrEqual Operator = ">="
	In                 Operator = "in"
	NotIn              Operator = "not-in"
	ArrayContains      Operator = "array-contains"
	ArrayContainsAny   Operator = "array-contains-any"
)

// IsSupported reports whether op is one of Firego's supported comparison
// operators.
func (op Operator) IsSupported() bool {
	switch op {
	case Equal, NotEqual, LessThan, LessThanOrEqual, GreaterThan, GreaterThanOrEqual,
		In, NotIn, ArrayContains, ArrayContainsAny:
		return true
	default:
		return false
	}
}

// Filter is a single Firestore query condition: Field Op Value. Field is a
// Firestore document field name (a schema.Field's FirestoreName), not a Go
// struct field name — resolving the latter into the former is the caller's
// responsibility.
type Filter struct {
	Field string
	Op    Operator
	Value any
}

// Direction controls a Firestore query's sort direction.
type Direction string

const (
	Asc  Direction = "asc"
	Desc Direction = "desc"
)

// IsSupported reports whether direction is Asc or Desc.
func (direction Direction) IsSupported() bool {
	return direction == Asc || direction == Desc
}

// Order is one resolved Firestore sort field and direction.
type Order struct {
	Field     string
	Direction Direction
}

// CursorBound identifies which edge of a result set a Cursor bounds.
type CursorBound int

const (
	// StartAt includes the matching document (and everything after it).
	StartAt CursorBound = iota
	// StartAfter excludes the matching document, starting after it.
	StartAfter
	// EndAt includes the matching document (and everything before it).
	EndAt
	// EndBefore excludes the matching document, ending before it.
	EndBefore
)

// Cursor bounds a query's result set relative to a document position,
// matching Firestore's query cursor semantics. Values identifies a prefix of
// the query's OrderBy fields, in the same order: it must hold at least one
// value, and at most one value per OrderBy field.
type Cursor struct {
	Bound  CursorBound
	Values []any
}
