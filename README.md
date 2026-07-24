# Firego

Firego is an object-document mapper (ODM) for [Google Cloud Firestore](https://cloud.google.com/firestore) in Go.

The core idea: your model is a plain Go struct, and Firego takes care of the parts that are usually left to the caller — converting between Firestore's wire types and your struct's field types, and (eventually) wrapping multi-step operations in transactions. You describe the shape of your data with struct tags; Firego handles the rest.

> **Status: early development.** The schema/codec layer described below works and is tested, but there is no public client API for reading, writing, or querying documents yet, and transaction support has not been implemented. See [Project status](#project-status) for a package-by-package breakdown. The API shown in [Vision](#vision) is a design target, not a guarantee of the final shape.

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
- `firego:"id"` — marks the field that receives the Firestore document ID. It must be a `string` field, is never written into the document body, and is not populated by the codec itself (that wiring belongs to the client, once it exists).

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

- **Schema discovery** (`internal/metadata`): builds a `schema.Schema` for a model type from its struct tags, including embedded-field promotion and ID-field validation.
- **Codec** (`codec`): given a `schema.Schema`, encodes a struct into a `map[string]any` and decodes a `map[string]any` back into a struct, converting between compatible types (for example, Firestore's `int64` into a Go `int` field) while rejecting conversions that cross incompatible kind families (e.g. string into int).

These two packages are exercised by the test suite and are the foundation the client will be built on.

## Vision

The target shape of the public API — not yet implemented — looks roughly like this:

```go
client, err := firego.NewClient(ctx, projectID)
if err != nil {
	log.Fatal(err)
}

users := firego.Collection[User](client, "users")

// Reads and writes work with plain structs; no map[string]any, no manual
// type juggling.
u, err := users.Get(ctx, "user-123")

err = users.Set(ctx, User{ID: "user-123", Name: "Alice", Age: 30})

// Multi-step operations run inside a Firestore transaction without the
// caller writing RunTransaction themselves.
err = firego.RunTransaction(ctx, client, func(tx *firego.Tx) error {
	u, err := users.Tx(tx).Get(ctx, "user-123")
	if err != nil {
		return err
	}
	u.Age++
	return users.Tx(tx).Set(ctx, u)
})
```

## Project status

| Package             | Purpose                                             | Status                                  |
|----------------------|------------------------------------------------------|------------------------------------------|
| `schema`             | Describes the mapping between a Go type and a Firestore collection/fields | Implemented |
| `internal/metadata`  | Builds a `schema.Schema` from struct tags via reflection | Implemented (internal — not importable outside this module) |
| `codec`              | Encodes/decodes between structs and `map[string]any`, with type conversion | Implemented |
| `client`             | Wraps `*firestore.Client`                            | Skeleton only — no read/write/query methods yet |
| `query`              | Query building                                       | Not started |
| Transactions          | Automatic transaction wrapping for multi-step operations | Not started |

## Development

```bash
go test ./...
```

## License

No license has been chosen yet; the repository is not currently licensed for reuse.
