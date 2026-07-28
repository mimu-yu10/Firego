// Package query provides types for building Firestore queries.
package query

// Equal is the only comparison operator Filter currently supports.
const Equal = "=="

// Filter is a single Firestore query condition: Field Op Value. Field is a
// Firestore document field name (a schema.Field's FirestoreName), not a Go
// struct field name — resolving the latter into the former is the caller's
// responsibility.
type Filter struct {
	Field string
	Op    string
	Value any
}
