package membrane

import "encoding/json"

// TransformFunc converts a service payload to/from an EventGraph payload.
type TransformFunc func(in json.RawMessage) (json.RawMessage, error)

// TransformRegistry holds named transforms for event mapping.
type TransformRegistry struct {
	transforms map[string]TransformFunc
}

// NewTransformRegistry creates an empty registry.
func NewTransformRegistry() *TransformRegistry {
	return &TransformRegistry{transforms: make(map[string]TransformFunc)}
}

// Register adds a named transform.
func (r *TransformRegistry) Register(name string, fn TransformFunc) {
	r.transforms[name] = fn
}

// Get retrieves a transform by name.
func (r *TransformRegistry) Get(name string) (TransformFunc, bool) {
	fn, ok := r.transforms[name]
	return fn, ok
}

// PassthroughTransform returns the input unchanged.
func PassthroughTransform(in json.RawMessage) (json.RawMessage, error) {
	return in, nil
}
