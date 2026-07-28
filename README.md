# Firego

Firego is an object-document mapper (ODM) for [Google Cloud Firestore](https://cloud.google.com/firestore) in Go.

The core idea: your model is a plain Go struct, and Firego takes care of the parts that are usually left to the caller — converting between Firestore's wire types and your struct's field types, and (eventually) wrapping multi-step operations in transactions. You describe the shape of your data with struct tags; Firego handles the rest.

> **Status: early development.** Reading and writing a single document by ID works — see [Usage](#usage). Querying and transactions are not implemented yet. See [Project status](#project-status) for a package-by-package breakdown. The transaction API shown in [Vision](#vision) is a design target, not a guarantee of the final shape.

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
- **Client** (`client`): wraps a `*firestore.Client` with a per-model schema cache and `GetDocument`/`SetDocument` methods that read and write raw document data, mapping a missing document to `ErrNotFound`.
- **Firego** (`firego`, the top-level package): the public entry point. `NewClient` creates a client; `Collection[T]` returns a type-safe `CollectionRef[T]` whose `Get` and `Set` methods handle encoding, decoding, and ID-field wiring so callers only deal with plain structs.

These packages are exercised by the test suite. `Get`/`Set`'s encode/decode/error-propagation logic is covered end-to-end against an in-memory fake; the thin adapter that calls the real Firestore SDK is not yet covered by an automated test beyond its NotFound-mapping logic, since no Firestore emulator is available in this environment yet.

## Usage

```go
client, err := firego.NewClient(ctx, projectID)
if err != nil {
	log.Fatal(err)
}

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
```

## Vision

Transactions are not implemented yet. The target shape — not a guarantee of the final API — looks roughly like this:

```go
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
| `codec`              | Encodes/decodes between structs and `map[string]any`, with type conversion and ID-field access | Implemented |
| `client`             | Wraps `*firestore.Client`; per-model schema cache; `GetDocument`/`SetDocument` | Implemented |
| `firego` (top-level) | Public API: `NewClient`, `Collection[T]`, `Get`, `Set`  | Implemented |
| `query`              | Query building                                       | Not started |
| Transactions          | Automatic transaction wrapping for multi-step operations | Not started |

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

No license has been chosen yet; the repository is not currently licensed for reuse.
