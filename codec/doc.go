// Package codec encodes Go models into Firestore documents and decodes
// Firestore documents back into Go models.
package codec

type Codec interface {
	Encode(v any) (map[string]any, error)
	Decode(data map[string]any, dst any) error

	// ID returns the current value of v's ID field, following the same
	// calling convention as Encode (v may be a value or a pointer to one).
	// It returns "" if the schema declares no ID field.
	ID(v any) (string, error)

	// SetID writes id into dst's ID field, following the same calling
	// convention as Decode (dst must be a non-nil pointer). It is a no-op
	// if the schema declares no ID field.
	SetID(dst any, id string) error
}
