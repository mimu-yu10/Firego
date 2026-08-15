# Firego

Firego is an object-document mapper (ODM) for [Google Cloud Firestore](https://cloud.google.com/firestore) in Go.

The core idea: your model is a plain Go struct, and Firego takes care of the parts that are usually left to the caller — converting between Firestore's wire types and your struct's field types, and wrapping multi-step operations in transactions. You describe the shape of your data with struct tags; Firego handles the rest.

> **Status: early development.** Reading and writing a single document by ID works, as do filters, ordering, limits, cursor pagination, and transactions — see [Usage](#usage). See [Project status](#project-status) for a package-by-package breakdown.

## Why Firego

Working with the official Firestore Go client directly usually means writing the same boilerplate in every project:

- Manually converting Firestore's returned types (e.g. `int64` for any whole number) into the concrete numeric type your struct actually uses.
- Remembering to exclude the document ID from the data you write back, and re-attaching it after a read.
- Wrapping related reads and writes in a `RunTransaction` closure by hand, every time, in every call site.

Firego's goal is to move that bookkeeping into the library so application code only deals with plain structs.

## Installation

```bash
go get github.com/mimu-y10/firego
```

Requires Go 1.26 or later.

## Defining a model

Models are plain structs annotated with two optional tags:

- `firestore:"name"` — sets the field's name in the Firestore document. Defaults to the Go field name. Use `firestore:"-"` to exclude a field entirely.
- `firego:"id"` — marks the field that receives the Firestore document ID. It must be a `string` field, is never written into the document body, and is populated automatically by `Collection[T].Get` after decoding.

```go
type User struct {
	ID        string    `firego:"id" firestore:"-"`
	Name      string    `firestore:"name"`
	Age       int
	CreatedAt time.Time `firestore:"created_at"`
}
```

Embedded structs are promoted into the parent's field list — matching `encoding/json` and the official Firestore SDK — unless the embedded field carries an explicit `firestore` tag name, in which case it is kept as a single nested field instead.

## What works today

- **Schema discovery** (`internal/metadata`): builds a `schema.Schema` for a model type from its struct tags, including embedded-field promotion and ID-field validation. A `Registry` caches the resulting schema per model type and collection, so repeated lookups for the same pair skip reflection after the first call.
- **Codec** (`codec`): given a `schema.Schema`, encodes a struct into a `map[string]any` and decodes a `map[string]any` back into a struct, converting between compatible types (for example, Firestore's `int64` into a Go `int` field) while rejecting conversions that cross incompatible kind families (e.g. string into int). Also reads and writes the ID field (`ID`/`SetID`), independently of the document body.
- **Query** (`query`): the typed comparison operators and `Filter` type shared by the root package's query implementation.
- **Firego** (`firego`, the top-level package): the public entry point and Firestore client wrapper. `NewClient` creates a client with a per-model schema cache; `Collection[T]` returns a type-safe `CollectionRef[T]` whose `Get`, `Set`, `Create`, and `Delete` methods handle encoding, decoding, and ID-field wiring so callers only deal with plain structs. `CollectionRef[T].Where` starts a `Query[T]`, an immutable, chainable equality filter that `Documents` runs, decoding every match into a `[]T` with each result's ID field populated the same way `Get` populates it. `RunTransaction` wraps a callback in a Firestore transaction; `CollectionRef[T].Tx` returns a `TxCollectionRef[T]` with the same `Get`/`Set`/`Create`/`Delete`/`Update` methods, scoped to that transaction.

These packages are exercised by the test suite. `Get`/`Set`/`Where`/`Tx`'s encode/decode/error-propagation logic is covered end-to-end against an in-memory fake; the thin adapter that calls the real Firestore SDK is not yet covered by an automated test beyond its NotFound-mapping logic, since no Firestore emulator is available in this environment yet.

## Usage

```go
client, err := firego.NewClient(ctx, projectID)
if err != nil {
	log.Fatal(err)
}

// To use a preconfigured Firestore client (for example, for a named
// database), wrap it instead:
// client, err := firego.NewClientFromFirestore(firestoreClient)

users, err := firego.Collection[User](client, "users")
if err != nil {
	log.Fatal(err)
}

// Reads and writes work with plain structs; no map[string]any, no manual
// type juggling. Get populates User.ID with the document ID automatically.
u, err := users.Get(ctx, "user-123")

// Set requires the model's ID field to already be populated — Firego does
// not auto-generate document IDs.
err = users.Set(ctx, User{ID: "user-123", Name: "Alice", Age: 30})

// Create requires the document not to exist already.
err = users.Create(ctx, User{ID: "user-456", Name: "Bob", Age: 24})

// Delete is idempotent; deleting a missing document also succeeds.
err = users.Delete(ctx, "user-456")

// Update resolves Go field names through the model schema.
err = users.Update(ctx, "user-123", firego.FieldUpdate{Field: "Age", Value: 31})

// Where filters on a Go struct field name (not its Firestore name) and
// matches on equality. Chained Where calls combine with AND. Each result's
// ID field is populated the same way Get populates it.
adults, err := users.Where("Age", 30).Documents(ctx)

// Use WhereOp for non-equality comparisons.
adults, err = users.WhereOp("Age", query.GreaterThanOrEqual, 18).Documents(ctx)

// Membership and array operators use the same builder.
selected, err := users.WhereOp("Age", query.In, []int{20, 30}).Documents(ctx)

// OrderBy calls are immutable and may be chained after filters.
youngestFirst, err := users.Where("Active", true).OrderBy("Age", query.Asc).Documents(ctx)

// Limit caps the number of returned documents. Cursor pagination needs a
// tie-breaker that's actually unique across the collection, or ties at the
// page boundary can be split across pages inconsistently — Age alone isn't
// unique, and neither is CreatedAt on its own if your writes can share a
// timestamp; use whatever field (or combination) your schema genuinely
// guarantees is unique. This example assumes CreatedAt is.
firstPage, err := users.OrderBy("Age", query.Asc).OrderBy("CreatedAt", query.Asc).Limit(20).Documents(ctx)

// StartAfter fetches the next page following the last result of a previous
// page. Values must match the query's OrderBy fields, in order.
last := firstPage[len(firstPage)-1]
nextPage, err := users.OrderBy("Age", query.Asc).OrderBy("CreatedAt", query.Asc).
	StartAfter(last.Age, last.CreatedAt).Limit(20).Documents(ctx)

// RunTransaction wraps a multi-step read-modify-write in a Firestore
// transaction. CollectionRef[T].Tx swaps in the transaction-scoped Get/Set;
// all reads through tx must happen before any writes, per Firestore's rules.
// Tx returns an error if tx wasn't opened on the same Client users was
// created from.
err = firego.RunTransaction(ctx, client, func(ctx context.Context, tx *firego.Tx) error {
	usersTx, err := users.Tx(tx)
	if err != nil {
		return err
	}
	u, err := usersTx.Get(ctx, "user-123")
	if err != nil {
		return err
	}
	u.Age++
	return usersTx.Set(ctx, u)
})
```

## Project status

| Package             | Purpose                                             | Status                                  |
|----------------------|------------------------------------------------------|------------------------------------------|
| `schema`             | Describes the mapping between a Go type and a Firestore collection/fields | Implemented |
| `internal/metadata`  | Builds a `schema.Schema` from struct tags via reflection | Implemented (internal — not importable outside this module) |
| `codec`              | Encodes/decodes between structs and `map[string]any`, with type conversion and ID-field access | Implemented |
| `firego` (top-level) | Firestore client wrapper and public API: `NewClient`, `Collection[T]`, `Get`, `Set`, `Create`, `Update`, `Delete`, `Where`/`Documents` | Implemented |
| `query`              | Query building                                       | Filters, ordering, limits, and cursor pagination (`StartAt`/`StartAfter`/`EndAt`/`EndBefore`) |
| Transactions          | Automatic transaction wrapping for multi-step operations | Implemented: `RunTransaction`, `CollectionRef[T].Tx`. Querying (`Where`/`Documents`) within a transaction is not supported. |

## Development

CI tasks (lint, build, test) run through [mage](https://magefile.org). No local mage install is required — invoke it via `go run`:

```bash
go run github.com/magefile/mage@v1.17.2 lint   # golangci-lint
go run github.com/magefile/mage@v1.17.2 build  # go build ./...
go run github.com/magefile/mage@v1.17.2 test   # go test -race ./...
go run github.com/magefile/mage@v1.17.2 ci     # runs all three
```

If you have mage installed (`go install github.com/magefile/mage@latest`), the same targets are available as `mage lint`, `mage build`, `mage test`, and `mage ci`.

## License

MIT — see [LICENSE](LICENSE).
