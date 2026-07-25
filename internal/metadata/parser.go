package metadata

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mimu-y10/firego/schema"
)

// Parse creates schema metadata for model T and the given collection.
func Parse[T any](collection string) (*schema.Schema, error) {
	if collection == "" {
		return nil, fmt.Errorf("metadata: collection must not be empty")
	}

	modelType := resolveModelType[T]()

	if modelType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("metadata: model must be a struct")
	}

	fields, err := collectFields(modelType, nil)
	if err != nil {
		return nil, err
	}

	idFieldIndex := -1
	for i, f := range fields {
		if f.IsID {
			if idFieldIndex >= 0 {
				return nil, fmt.Errorf("metadata: multiple ID fields")
			}
			idFieldIndex = i
		}
	}

	result := &schema.Schema{
		Name:       modelType.Name(),
		Collection: collection,
		GoType:     modelType,
		Fields:     fields,
	}

	if idFieldIndex >= 0 {
		result.IDField = &result.Fields[idFieldIndex]
	}

	return result, nil
}

// resolveModelType returns the struct type underlying T, unwrapping any
// number of pointer indirections (T, *T, **T, ...).
func resolveModelType[T any]() reflect.Type {
	modelType := reflect.TypeFor[T]()
	for modelType.Kind() == reflect.Pointer {
		modelType = modelType.Elem()
	}
	return modelType
}

// collectFields walks t and returns schema.Field entries for every mapped
// field. Anonymous (embedded) struct fields whose firestore tag carries no
// explicit name are promoted — their inner fields are appended directly, with
// StructIndex extended by the embedding position — matching the behaviour of
// the Firestore Go SDK encoder/decoder.
func collectFields(t reflect.Type, indexPrefix []int) ([]schema.Field, error) {
	fields := make([]schema.Field, 0, t.NumField())

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)

		// Build the full reflection path to this field.
		structIndex := append(append([]int(nil), indexPrefix...), i)

		// Anonymous (embedded) fields: handle before the exported check.
		// Unexported anonymous struct types are promoted just like exported
		// ones — matching encoding/json and Firestore SDK behaviour — because
		// their own exported fields remain accessible on the outer struct.
		if sf.Anonymous {
			embeddedType := sf.Type
			for embeddedType.Kind() == reflect.Pointer {
				embeddedType = embeddedType.Elem()
			}
			tagName, _, _ := strings.Cut(sf.Tag.Get("firestore"), ",")
			// Promote when no explicit firestore name is given.
			// Unexported anonymous types can never carry a meaningful tag, so
			// also promote them even if a tag is present.
			if embeddedType.Kind() == reflect.Struct && (tagName == "" || !sf.IsExported()) {
				promoted, err := collectFields(embeddedType, structIndex)
				if err != nil {
					return nil, err
				}
				fields = append(fields, promoted...)
				continue
			}
		}

		if !sf.IsExported() {
			continue
		}

		isID := sf.Tag.Get("firego") == "id"
		firestoreName := parseFirestoreName(sf)

		if firestoreName == "-" && !isID {
			continue
		}

		if isID && sf.Type.Kind() != reflect.String {
			return nil, fmt.Errorf("metadata: ID field %q must be a string", sf.Name)
		}

		fields = append(fields, schema.Field{
			Name:          sf.Name,
			FirestoreName: firestoreName,
			GoType:        sf.Type,
			StructIndex:   structIndex,
			IsID:          isID,
		})
	}

	return fields, nil
}

func parseFirestoreName(field reflect.StructField) string {
	tag := field.Tag.Get("firestore")
	if tag == "" {
		return field.Name
	}

	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return field.Name
	}

	return name
}
