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

// ErrUnsupportedDirection is returned when OrderBy receives a direction that
// Firego does not support.
var ErrUnsupportedDirection = errors.New("firego: unsupported query direction")

// ErrInvalidLimit is returned when Limit receives a negative value.
var ErrInvalidLimit = errors.New("firego: query limit must not be negative")

// ErrIDFieldNotQueryable is returned by (*Query[T]).Documents when a Where
// call named the model's ID field. A document's ID is not part of its
// Firestore data, so it cannot be filtered on — use Get with the ID
// directly instead.
var ErrIDFieldNotQueryable = errors.New("firego: ID field cannot be queried, use Get instead")

// ErrIDFieldNotOrderable is returned when OrderBy names the model's ID field.
var ErrIDFieldNotOrderable = errors.New("firego: ID field ordering is not supported")

// Query is a Firestore query against a CollectionRef[T]'s collection, built
// by chaining filters and ordering. A Query is immutable: every builder
// returns a new Query rather than modifying the receiver, so a base Query can
// be safely reused to build several different queries.
type Query[T any] struct {
	ref     *CollectionRef[T]
	filters []query.Filter
	orders  []query.Order
	limit   *int
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

// OrderBy starts a Query ordered by field in direction. field names a Go
// struct field and is resolved to its Firestore name when the query runs.
func (r *CollectionRef[T]) OrderBy(field string, direction query.Direction) *Query[T] {
	return (&Query[T]{ref: r}).appendOrder(field, direction)
}

// OrderBy returns a Query with an additional ordering, leaving q unmodified.
// Multiple calls apply ordering from first to last.
func (q *Query[T]) OrderBy(field string, direction query.Direction) *Query[T] {
	return q.appendOrder(field, direction)
}

// Limit starts a Query that returns at most n documents. A zero limit is
// valid and returns no documents.
func (r *CollectionRef[T]) Limit(n int) *Query[T] {
	return (&Query[T]{ref: r}).withLimit(n)
}

// Limit returns a Query with its maximum result count set to n, leaving q
// unmodified. A later Limit call replaces the previous value.
func (q *Query[T]) Limit(n int) *Query[T] {
	return q.withLimit(n)
}

// appendFilter returns a new Query whose filters are q's filters plus one
// comparison, leaving q itself unmodified.
func (q *Query[T]) appendFilter(field string, op query.Operator, value any) *Query[T] {
	filters := make([]query.Filter, len(q.filters), len(q.filters)+1)
	copy(filters, q.filters)
	filters = append(filters, query.Filter{Field: field, Op: op, Value: value})
	return &Query[T]{ref: q.ref, filters: filters, orders: q.orders, limit: q.limit}
}

func (q *Query[T]) appendOrder(field string, direction query.Direction) *Query[T] {
	orders := make([]query.Order, len(q.orders), len(q.orders)+1)
	copy(orders, q.orders)
	orders = append(orders, query.Order{Field: field, Direction: direction})
	return &Query[T]{ref: q.ref, filters: q.filters, orders: orders, limit: q.limit}
}

func (q *Query[T]) withLimit(n int) *Query[T] {
	return &Query[T]{ref: q.ref, filters: q.filters, orders: q.orders, limit: &n}
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
	if q.limit != nil && *q.limit < 0 {
		return nil, fmt.Errorf("firego: query %s: limit %d: %w", r.schema.Collection, *q.limit, ErrInvalidLimit)
	}

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
	resolvedOrders := make([]query.Order, len(q.orders))
	for i, order := range q.orders {
		if !order.Direction.IsSupported() {
			return nil, fmt.Errorf("firego: query %s: direction %q: %w", r.schema.Collection, order.Direction, ErrUnsupportedDirection)
		}
		sf, ok := r.schema.FieldByName(order.Field)
		if !ok {
			return nil, fmt.Errorf("firego: query %s: order field %q: %w", r.schema.Collection, order.Field, ErrUnknownField)
		}
		if sf.IsID {
			return nil, fmt.Errorf("firego: query %s: order field %q: %w", r.schema.Collection, order.Field, ErrIDFieldNotOrderable)
		}
		resolvedOrders[i] = query.Order{Field: sf.FirestoreName, Direction: order.Direction}
	}

	docs, err := r.store.queryDocuments(ctx, r.schema.Collection, resolved, resolvedOrders, q.limit)
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
