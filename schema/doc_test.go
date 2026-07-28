package schema

import (
	"reflect"
	"testing"
)

type embeddedAudit struct {
	Name string
}

// modelWithShadowedField declares a top-level Name field alongside an
// embedded struct that also has a Name field. Go's field-selection rules
// resolve Model{}.Name to the shallower, top-level field.
type modelWithShadowedField struct {
	embeddedAudit
	Name string
}

func TestFieldByNamePrefersShallowerFieldOverPromoted(t *testing.T) {
	s := &Schema{
		GoType: reflect.TypeFor[modelWithShadowedField](),
		Fields: []Field{
			// Promoted from embeddedAudit; appears first because the
			// metadata parser visits embedded fields before top-level ones.
			{Name: "Name", FirestoreName: "embedded_name", StructIndex: []int{0, 0}},
			{Name: "Name", FirestoreName: "name", StructIndex: []int{1}},
		},
	}

	f, ok := s.FieldByName("Name")
	if !ok {
		t.Fatal(`FieldByName("Name") not found`)
	}
	if f.FirestoreName != "name" {
		t.Errorf(`FieldByName("Name").FirestoreName = %q, want %q (the top-level field, not the one promoted from embeddedAudit)`, f.FirestoreName, "name")
	}
}

func TestFieldByNameReturnsFalseForUnknownField(t *testing.T) {
	s := &Schema{
		GoType: reflect.TypeFor[modelWithShadowedField](),
		Fields: []Field{
			{Name: "Name", FirestoreName: "name", StructIndex: []int{1}},
		},
	}

	if _, ok := s.FieldByName("Nonexistent"); ok {
		t.Error(`FieldByName("Nonexistent") found, want not found`)
	}
}

func TestFieldByNameFindsOrdinaryField(t *testing.T) {
	type plain struct {
		Age int
	}
	s := &Schema{
		GoType: reflect.TypeFor[plain](),
		Fields: []Field{
			{Name: "Age", FirestoreName: "Age", StructIndex: []int{0}},
		},
	}

	f, ok := s.FieldByName("Age")
	if !ok {
		t.Fatal(`FieldByName("Age") not found`)
	}
	if f.FirestoreName != "Age" {
		t.Errorf(`FieldByName("Age").FirestoreName = %q, want %q`, f.FirestoreName, "Age")
	}
}
