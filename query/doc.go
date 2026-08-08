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
)

// IsSupported reports whether op is one of Firego's supported comparison
// operators.
func (op Operator) IsSupported() bool {
	switch op {
	case Equal, NotEqual, LessThan, LessThanOrEqual, GreaterThan, GreaterThanOrEqual:
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
