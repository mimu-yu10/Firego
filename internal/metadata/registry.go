package metadata

import (
	"reflect"
	"sync"

	"github.com/mimu-y10/firego/schema"
)

// Registry caches schema.Schema values built by Parse, keyed by model type
// and collection, so repeated lookups for the same (T, collection) pair
// avoid re-walking the struct with reflection on every call.
//
// The zero value is ready to use. A Registry is safe for concurrent use.
type Registry struct {
	schemas sync.Map // cacheKey -> *schema.Schema
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// cacheKey identifies one cached schema. Two Get calls only share a cache
// entry when both the resolved model type and the collection match.
type cacheKey struct {
	modelType  reflect.Type
	collection string
}

// Get returns the schema for model T and collection, building it with Parse
// on the first call for that (T, collection) pair and returning the cached
// result on every subsequent call. Errors are never cached, since they are
// cheap to reproduce and caching them could hide a since-fixed model.
//
// Concurrent calls that race on the same (T, collection) pair may each call
// Parse, but all of them return the same cached *schema.Schema afterward.
func Get[T any](r *Registry, collection string) (*schema.Schema, error) {
	key := cacheKey{modelType: resolveModelType[T](), collection: collection}

	if cached, ok := r.schemas.Load(key); ok {
		return cached.(*schema.Schema), nil
	}

	s, err := Parse[T](collection)
	if err != nil {
		return nil, err
	}

	actual, _ := r.schemas.LoadOrStore(key, s)
	return actual.(*schema.Schema), nil
}
