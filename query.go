package firego

import (
	"context"
	"errors"
	"fmt"

	"github.com/mimu-y10/firego/query"
)

// ErrUnknownField is returned by (*Query[T]).Documents when a Where call
// named a field the model does not declare.
var ErrUnknownField = errors.New("firego: unknown field")

// ErrUnsupportedOperator is returned when WhereOp receives an operator that
// Firego does not support.
var ErrUnsupportedOperator = errors.New("firego: unsupported query operator")

// ErrIDFieldNotQueryable is returned by (*Query[T]).Documents when a Where
// call named the model's ID field. A document's ID is not part of its
// Firestore data, so it cannot be filtered on — use Get with the ID
// directly instead.
var ErrIDFieldNotQueryable = errors.New("firego: ID field cannot be queried, use Get instead")

// Query is a Firestore query against a CollectionRef[T]'s collection, built
// by chaining Where calls. A Query is immutable: every Where call returns a
// new Query rather than modifying the receiver, so a base Query can be
// safely reused to build several different queries.
type Query[T any] struct {
	ref     *CollectionRef[T]
	filters []query.Filter
}

// Where returns a Query that additionally requires field to equal value.
// field must name a Go struct field declared on the model (not its
// Firestore name); it is resolved against the model's schema when the
// query runs. Multiple Where calls combine with AND:
//
//	adults := users.Where("Age", 18).Where("Active", true)
func (r *CollectionRef[T]) Where(field string, value any) *Query[T] {
	return (&Query[T]{ref: r}).appendFilter(field, query.Equal, value)
}

// WhereOp returns a Query that compares field to value using op. field names
// a Go struct field; supported operators are declared by package query.
func (r *CollectionRef[T]) WhereOp(field string, op query.Operator, value any) *Query[T] {
	return (&Query[T]{ref: r}).appendFilter(field, op, value)
}

// Where returns a Query that additionally requires field to equal value,
// combined with q's existing filters using AND. See CollectionRef[T].Where
// for details.
func (q *Query[T]) Where(field string, value any) *Query[T] {
	return q.appendFilter(field, query.Equal, value)
}

// WhereOp returns a Query that additionally compares field to value using op,
// combined with q's existing filters using AND.
func (q *Query[T]) WhereOp(field string, op query.Operator, value any) *Query[T] {
	return q.appendFilter(field, op, value)
}

// appendFilter returns a new Query whose filters are q's filters plus one
// comparison, leaving q itself unmodified.
func (q *Query[T]) appendFilter(field string, op query.Operator, value any) *Query[T] {
	filters := make([]query.Filter, len(q.filters), len(q.filters)+1)
	copy(filters, q.filters)
	filters = append(filters, query.Filter{Field: field, Op: op, Value: value})
	return &Query[T]{ref: q.ref, filters: filters}
}

// Documents runs the query and decodes every matching document into a T,
// with each result's ID field populated the same way Get populates it.
//
// It returns an error satisfying errors.Is(err, ErrUnknownField) if a Where
// call named a field the model does not declare, or
// errors.Is(err, ErrIDFieldNotQueryable) if a Where call named the model's
// ID field.
func (q *Query[T]) Documents(ctx context.Context) ([]T, error) {
	r := q.ref

	resolved := make([]query.Filter, len(q.filters))
	for i, f := range q.filters {
		if !f.Op.IsSupported() {
			return nil, fmt.Errorf("firego: query %s: operator %q: %w", r.schema.Collection, f.Op, ErrUnsupportedOperator)
		}
		sf, ok := r.schema.FieldByName(f.Field)
		if !ok {
			return nil, fmt.Errorf("firego: query %s: field %q: %w", r.schema.Collection, f.Field, ErrUnknownField)
		}
		if sf.IsID {
			return nil, fmt.Errorf("firego: query %s: field %q: %w", r.schema.Collection, f.Field, ErrIDFieldNotQueryable)
		}
		resolved[i] = query.Filter{Field: sf.FirestoreName, Op: f.Op, Value: f.Value}
	}

	docs, err := r.store.queryDocuments(ctx, r.schema.Collection, resolved)
	if err != nil {
		return nil, fmt.Errorf("firego: query %s: %w", r.schema.Collection, err)
	}

	results := make([]T, len(docs))
	for i, d := range docs {
		if err := r.codec.Decode(d.Data, &results[i]); err != nil {
			return nil, fmt.Errorf("firego: query %s/%s: decode: %w", r.schema.Collection, d.ID, err)
		}
		if err := r.codec.SetID(&results[i], d.ID); err != nil {
			return nil, fmt.Errorf("firego: query %s/%s: set id: %w", r.schema.Collection, d.ID, err)
		}
	}
	return results, nil
}
