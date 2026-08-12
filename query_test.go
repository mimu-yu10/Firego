package firego

import (
	"context"
	"errors"
	"testing"

	"github.com/mimu-y10/firego/query"
)

func TestWhereFiltersAndDecodes(t *testing.T) {
	store := newFakeStore()
	store.docs["users/abc"] = map[string]any{"name": "Alice", "Age": 30}
	store.docs["users/def"] = map[string]any{"name": "Bob", "Age": 25}
	ref := newTestRef[testUser](t, store, "users")

	got, err := ref.Where("Age", 30).Documents(context.Background())
	if err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	want := []testUser{{ID: "abc", Name: "Alice", Age: 30}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("Documents() = %+v, want %+v", got, want)
	}
}

func TestWhereResolvesGoFieldNameToFirestoreName(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	if _, err := ref.Where("Name", "Alice").Documents(context.Background()); err != nil {
		t.Fatalf("Documents() error = %v", err)
	}

	if store.lastQueryCollection != "users" {
		t.Errorf("queryDocuments collection = %q, want %q", store.lastQueryCollection, "users")
	}
	if len(store.lastQueryFilters) != 1 {
		t.Fatalf("queryDocuments filters = %v, want exactly 1 filter", store.lastQueryFilters)
	}
	// "Name" is the Go field; its FirestoreName is "name" per the
	// `firestore:"name"` tag on testUser.
	if got := store.lastQueryFilters[0].Field; got != "name" {
		t.Errorf("filter field = %q, want %q (the FirestoreName, not the Go field name)", got, "name")
	}
}

func TestWhereChainCombinesFiltersWithAnd(t *testing.T) {
	store := newFakeStore()
	store.docs["users/abc"] = map[string]any{"name": "Alice", "Age": 30}
	store.docs["users/def"] = map[string]any{"name": "Alice", "Age": 25}
	ref := newTestRef[testUser](t, store, "users")

	got, err := ref.Where("Name", "Alice").Where("Age", 30).Documents(context.Background())
	if err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "abc" {
		t.Errorf("Documents() = %+v, want exactly the abc document", got)
	}
	if len(store.lastQueryFilters) != 2 {
		t.Fatalf("queryDocuments filters = %v, want exactly 2 filters", store.lastQueryFilters)
	}
}

func TestWhereOpForwardsComparisonOperators(t *testing.T) {
	tests := []query.Operator{
		query.NotEqual,
		query.LessThan,
		query.LessThanOrEqual,
		query.GreaterThan,
		query.GreaterThanOrEqual,
	}

	for _, op := range tests {
		t.Run(string(op), func(t *testing.T) {
			store := newFakeStore()
			ref := newTestRef[testUser](t, store, "users")

			if _, err := ref.WhereOp("Age", op, 30).Documents(context.Background()); err != nil {
				t.Fatalf("Documents() error = %v", err)
			}
			if len(store.lastQueryFilters) != 1 {
				t.Fatalf("queryDocuments filters = %v, want exactly 1 filter", store.lastQueryFilters)
			}
			if got := store.lastQueryFilters[0].Op; got != op {
				t.Errorf("filter operator = %q, want %q", got, op)
			}
		})
	}
}

func TestWhereOpForwardsMembershipOperators(t *testing.T) {
	tests := []struct {
		op    query.Operator
		value any
	}{
		{op: query.In, value: []int{20, 30}},
		{op: query.NotIn, value: []int{20, 30}},
		{op: query.ArrayContains, value: 30},
		{op: query.ArrayContainsAny, value: []int{20, 30}},
	}

	for _, tt := range tests {
		t.Run(string(tt.op), func(t *testing.T) {
			store := newFakeStore()
			ref := newTestRef[testUser](t, store, "users")

			if _, err := ref.WhereOp("Age", tt.op, tt.value).Documents(context.Background()); err != nil {
				t.Fatalf("Documents() error = %v", err)
			}
			if len(store.lastQueryFilters) != 1 || store.lastQueryFilters[0].Op != tt.op {
				t.Fatalf("queryDocuments filters = %v, want operator %q", store.lastQueryFilters, tt.op)
			}
		})
	}
}

func TestWhereOpRejectsUnsupportedOperator(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	_, err := ref.WhereOp("Age", query.Operator("approximately"), 30).Documents(context.Background())
	if !errors.Is(err, ErrUnsupportedOperator) {
		t.Fatalf("Documents() error = %v, want errors.Is(err, ErrUnsupportedOperator)", err)
	}
}

func TestQueryIsImmutable(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	base := ref.Where("Name", "Alice")
	if _, err := base.Where("Age", 30).Documents(context.Background()); err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	if len(store.lastQueryFilters) != 2 {
		t.Fatalf("first branch filters = %v, want exactly 2 filters", store.lastQueryFilters)
	}

	// Reusing base for a second, different filter must not see the first
	// branch's Age filter leak in.
	if _, err := base.Where("Age", 99).Documents(context.Background()); err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	if len(store.lastQueryFilters) != 2 {
		t.Fatalf("second branch filters = %v, want exactly 2 filters", store.lastQueryFilters)
	}
	if got := store.lastQueryFilters[1].Value; got != 99 {
		t.Errorf("second branch Age filter = %v, want 99 (base must be unmodified by the first branch)", got)
	}
}

func TestOrderByResolvesFieldAndDirection(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	if _, err := ref.OrderBy("Name", query.Desc).Documents(context.Background()); err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	if len(store.lastQueryOrders) != 1 {
		t.Fatalf("queryDocuments orders = %v, want exactly 1 order", store.lastQueryOrders)
	}
	want := query.Order{Field: "name", Direction: query.Desc}
	if got := store.lastQueryOrders[0]; got != want {
		t.Errorf("queryDocuments order = %+v, want %+v", got, want)
	}
}

func TestOrderByIsImmutable(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")
	base := ref.Where("Age", 30)

	if _, err := base.OrderBy("Name", query.Asc).Documents(context.Background()); err != nil {
		t.Fatalf("ordered Documents() error = %v", err)
	}
	if _, err := base.Documents(context.Background()); err != nil {
		t.Fatalf("base Documents() error = %v", err)
	}
	if len(store.lastQueryOrders) != 0 {
		t.Errorf("base query orders = %v, want none", store.lastQueryOrders)
	}
}

func TestOrderByRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		direction query.Direction
		want      error
	}{
		{name: "unknown field", field: "Missing", direction: query.Asc, want: ErrUnknownField},
		{name: "ID field", field: "ID", direction: query.Asc, want: ErrIDFieldNotOrderable},
		{name: "direction", field: "Age", direction: query.Direction("sideways"), want: ErrUnsupportedDirection},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			ref := newTestRef[testUser](t, store, "users")
			_, err := ref.OrderBy(tt.field, tt.direction).Documents(context.Background())
			if !errors.Is(err, tt.want) {
				t.Fatalf("Documents() error = %v, want errors.Is(err, %v)", err, tt.want)
			}
		})
	}
}

func TestLimitRestrictsResults(t *testing.T) {
	store := newFakeStore()
	store.docs["users/a"] = map[string]any{"name": "Alice", "Age": 30}
	store.docs["users/b"] = map[string]any{"name": "Bob", "Age": 25}
	ref := newTestRef[testUser](t, store, "users")

	got, err := ref.Limit(1).Documents(context.Background())
	if err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Documents() returned %d results, want 1", len(got))
	}
	if store.lastQueryLimit == nil || *store.lastQueryLimit != 1 {
		t.Errorf("queryDocuments limit = %v, want 1", store.lastQueryLimit)
	}
}

func TestLimitIsImmutableAndReplaceable(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")
	base := ref.Where("Age", 30)

	if _, err := base.Limit(5).Limit(2).Documents(context.Background()); err != nil {
		t.Fatalf("limited Documents() error = %v", err)
	}
	if store.lastQueryLimit == nil || *store.lastQueryLimit != 2 {
		t.Fatalf("replacement limit = %v, want 2", store.lastQueryLimit)
	}
	if _, err := base.Documents(context.Background()); err != nil {
		t.Fatalf("base Documents() error = %v", err)
	}
	if store.lastQueryLimit != nil {
		t.Errorf("base query limit = %v, want nil", store.lastQueryLimit)
	}
}

func TestLimitRejectsNegativeValue(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	_, err := ref.Limit(-1).Documents(context.Background())
	if !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("Documents() error = %v, want errors.Is(err, ErrInvalidLimit)", err)
	}
}

func TestDocumentsRejectsUnknownField(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	_, err := ref.Where("Nickname", "Al").Documents(context.Background())
	if !errors.Is(err, ErrUnknownField) {
		t.Fatalf("Documents() error = %v, want errors.Is(err, ErrUnknownField)", err)
	}
}

func TestDocumentsRejectsIDField(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	_, err := ref.Where("ID", "abc").Documents(context.Background())
	if !errors.Is(err, ErrIDFieldNotQueryable) {
		t.Fatalf("Documents() error = %v, want errors.Is(err, ErrIDFieldNotQueryable)", err)
	}
}

func TestDocumentsPropagatesStoreError(t *testing.T) {
	store := newFakeStore()
	store.queryErr = errors.New("boom")
	ref := newTestRef[testUser](t, store, "users")

	if _, err := ref.Where("Age", 30).Documents(context.Background()); err == nil {
		t.Fatal("Documents() error = nil, want error")
	}
}

type embeddedCategory struct {
	Category string `firestore:"embedded_category"`
}

// testShadowedField declares a top-level Category field alongside an
// embedded struct that also has a Category field, so Where("Category", ...)
// exercises schema.FieldByName's use of Go's field-selection rules
// end-to-end: it must resolve to the top-level field, not the promoted one.
type testShadowedField struct {
	embeddedCategory
	ID       string `firego:"id" firestore:"-"`
	Category string `firestore:"top_category"`
}

func TestStartAfterResolvesValues(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	if _, err := ref.OrderBy("Age", query.Asc).StartAfter(30).Documents(context.Background()); err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	if store.lastQueryStart == nil {
		t.Fatal("queryDocuments start cursor = nil, want non-nil")
	}
	if store.lastQueryStart.Bound != query.StartAfter {
		t.Errorf("start cursor bound = %v, want %v", store.lastQueryStart.Bound, query.StartAfter)
	}
	if got := store.lastQueryStart.Values; len(got) != 1 || got[0] != 30 {
		t.Errorf("start cursor values = %v, want [30]", got)
	}
	if store.lastQueryEnd != nil {
		t.Errorf("end cursor = %v, want nil", store.lastQueryEnd)
	}
}

// TestCursorValuesAreCopied guards against StartAt/StartAfter/EndAt/EndBefore
// retaining the backing array of a caller-supplied slice expanded with "...".
// A Query is documented as immutable; without a copy, mutating the original
// slice after building the query would silently change it.
func TestCursorValuesAreCopied(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	values := []any{30}
	q := ref.OrderBy("Age", query.Asc).StartAfter(values...)
	values[0] = 999

	if _, err := q.Documents(context.Background()); err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	if got := store.lastQueryStart.Values; len(got) != 1 || got[0] != 30 {
		t.Errorf("start cursor values = %v, want [30] (unaffected by mutating the original slice)", got)
	}
}

func TestEndBeforeResolvesValues(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	if _, err := ref.OrderBy("Age", query.Asc).EndBefore(30).Documents(context.Background()); err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	if store.lastQueryEnd == nil {
		t.Fatal("queryDocuments end cursor = nil, want non-nil")
	}
	if store.lastQueryEnd.Bound != query.EndBefore {
		t.Errorf("end cursor bound = %v, want %v", store.lastQueryEnd.Bound, query.EndBefore)
	}
	if got := store.lastQueryEnd.Values; len(got) != 1 || got[0] != 30 {
		t.Errorf("end cursor values = %v, want [30]", got)
	}
}

func TestStartCursorReplacesPreviousStartCursor(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	q := ref.OrderBy("Age", query.Asc).StartAt(10).StartAfter(20)
	if _, err := q.Documents(context.Background()); err != nil {
		t.Fatalf("Documents() error = %v", err)
	}
	if store.lastQueryStart.Bound != query.StartAfter {
		t.Errorf("start cursor bound = %v, want %v (StartAfter should replace StartAt)", store.lastQueryStart.Bound, query.StartAfter)
	}
	if got := store.lastQueryStart.Values; len(got) != 1 || got[0] != 20 {
		t.Errorf("start cursor values = %v, want [20]", got)
	}
}

func TestCursorIsImmutable(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")
	base := ref.OrderBy("Age", query.Asc)

	if _, err := base.StartAfter(30).Documents(context.Background()); err != nil {
		t.Fatalf("cursor Documents() error = %v", err)
	}
	if _, err := base.Documents(context.Background()); err != nil {
		t.Fatalf("base Documents() error = %v", err)
	}
	if store.lastQueryStart != nil {
		t.Errorf("base query start cursor = %v, want nil", store.lastQueryStart)
	}
}

func TestCursorRejectsMissingOrderBy(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	_, err := ref.StartAfter(30).Documents(context.Background())
	if !errors.Is(err, ErrCursorRequiresOrderBy) {
		t.Fatalf("Documents() error = %v, want errors.Is(err, ErrCursorRequiresOrderBy)", err)
	}
}

// TestCursorAllowsOrderingPrefix exercises Firestore's field-value cursor
// semantics: a cursor's values identify a prefix of the ordered position, so
// supplying fewer values than there are OrderBy calls is valid (e.g.
// OrderBy("State").OrderBy("Population").StartAt("California")).
func TestCursorAllowsOrderingPrefix(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	_, err := ref.OrderBy("Age", query.Asc).OrderBy("Name", query.Asc).StartAfter(30).Documents(context.Background())
	if err != nil {
		t.Fatalf("Documents() error = %v, want nil (cursor values may cover an OrderBy prefix)", err)
	}
}

func TestCursorRejectsValueCountExceedingOrderBy(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	_, err := ref.OrderBy("Age", query.Asc).StartAfter(30, "Alice").Documents(context.Background())
	if !errors.Is(err, ErrCursorValueCount) {
		t.Fatalf("Documents() error = %v, want errors.Is(err, ErrCursorValueCount)", err)
	}
}

// TestCursorRejectsEmptyValues guards against a zero-length cursor (e.g.
// StartAfter() called with no arguments) reaching the store: it identifies
// no position at all, which Firestore could turn into an unbounded result or
// a backend error instead of a clear local validation failure.
func TestCursorRejectsEmptyValues(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testUser](t, store, "users")

	_, err := ref.OrderBy("Age", query.Asc).StartAfter().Documents(context.Background())
	if !errors.Is(err, ErrCursorValueCount) {
		t.Fatalf("Documents() error = %v, want errors.Is(err, ErrCursorValueCount)", err)
	}
}

func TestWhereResolvesShadowedFieldToTopLevel(t *testing.T) {
	store := newFakeStore()
	ref := newTestRef[testShadowedField](t, store, "items")

	if _, err := ref.Where("Category", "x").Documents(context.Background()); err != nil {
		t.Fatalf("Documents() error = %v", err)
	}

	if len(store.lastQueryFilters) != 1 {
		t.Fatalf("queryDocuments filters = %v, want exactly 1 filter", store.lastQueryFilters)
	}
	if got := store.lastQueryFilters[0].Field; got != "top_category" {
		t.Errorf("filter field = %q, want %q (the top-level field, not the one promoted from embeddedCategory)", got, "top_category")
	}
}
