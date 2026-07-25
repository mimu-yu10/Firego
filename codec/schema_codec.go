package codec

import (
	"fmt"
	"reflect"

	"github.com/mimu-y10/firego/schema"
)

// schemaCodec implements Codec using field metadata from a schema.Schema.
type schemaCodec struct {
	schema *schema.Schema
}

// New creates a Codec that encodes and decodes values of the type described
// by s. Fields marked as the ID field are excluded from the encoded and
// decoded document data, since a Firestore document ID is not part of its
// data.
func New(s *schema.Schema) Codec {
	return &schemaCodec{schema: s}
}

func (c *schemaCodec) Encode(v any) (map[string]any, error) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, fmt.Errorf("codec: cannot encode nil %s", rv.Type())
		}
		rv = rv.Elem()
	}
	if rv.Type() != c.schema.GoType {
		return nil, fmt.Errorf("codec: value type %s does not match schema type %s", rv.Type(), c.schema.GoType)
	}

	data := make(map[string]any, len(c.schema.Fields))
	for _, f := range c.schema.Fields {
		if f.IsID {
			continue
		}
		data[f.FirestoreName] = rv.FieldByIndex(f.StructIndex).Interface()
	}
	return data, nil
}

func (c *schemaCodec) Decode(data map[string]any, dst any) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("codec: dst must be a non-nil pointer")
	}
	rv = rv.Elem()
	if rv.Type() != c.schema.GoType {
		return fmt.Errorf("codec: dst type %s does not match schema type %s", rv.Type(), c.schema.GoType)
	}

	for _, f := range c.schema.Fields {
		if f.IsID {
			continue
		}
		raw, ok := data[f.FirestoreName]
		if !ok {
			continue
		}
		if err := setField(rv.FieldByIndex(f.StructIndex), raw); err != nil {
			return fmt.Errorf("codec: field %s: %w", f.Name, err)
		}
	}
	return nil
}

// setField assigns raw into fv, converting between compatible types (for
// example, Firestore's int64 into a Go int field).
func setField(fv reflect.Value, raw any) error {
	if raw == nil {
		fv.Set(reflect.Zero(fv.Type()))
		return nil
	}

	rv := reflect.ValueOf(raw)
	switch {
	case rv.Type().AssignableTo(fv.Type()):
		fv.Set(rv)
	case rv.Type().ConvertibleTo(fv.Type()) && sameKindFamily(rv.Kind(), fv.Kind()):
		fv.Set(rv.Convert(fv.Type()))
	default:
		return fmt.Errorf("cannot assign %s to %s", rv.Type(), fv.Type())
	}
	return nil
}

// sameKindFamily reports whether a and b belong to the same broad kind
// family (both numeric, both string, ...), guarding against reflect's
// permissive numeric<->string convertibility rules from causing silent,
// unwanted conversions.
func sameKindFamily(a, b reflect.Kind) bool {
	if isNumeric(a) && isNumeric(b) {
		return true
	}
	return a == b
}

func isNumeric(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}
