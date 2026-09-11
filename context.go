package ard

import "encoding/json"

// The IRIs of the description layer. See section 4.1.
const (
	BaseContextIRI   = "https://agenticresourcediscovery.org/context/v1"
	DefaultNamespace = "https://agenticresourcediscovery.org/ns#"
)

// Context is the JSON-LD context of an entry or a query. It holds a string, an object or
// an array, and keeps the raw form so that an encode gives back what a decode read.
type Context struct {
	raw json.RawMessage
}

// NewContext builds a context from a string, a map or a slice.
func NewContext(v any) (Context, error) { panic("ard: not implemented") }

// IsZero reports whether the document carried no context.
func (c Context) IsZero() bool { return len(c.raw) == 0 }

// Raw gives the context as it was written.
func (c Context) Raw() json.RawMessage { return c.raw }

func (c Context) MarshalJSON() ([]byte, error)  { panic("ard: not implemented") }
func (c *Context) UnmarshalJSON(b []byte) error { panic("ard: not implemented") }

// Prefixes gives the prefix to namespace bindings the context declares, and the vocab
// binding under the empty key.
func (c Context) Prefixes() (map[string]string, error) { panic("ard: not implemented") }

// BaseContext gives the ARD base context of section 4.1, embedded in the package.
func BaseContext() Context { panic("ard: not implemented") }
