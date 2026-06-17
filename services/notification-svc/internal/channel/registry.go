package channel

import (
	"fmt"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
)

// Registry maps channel type strings to their Channel implementations.
type Registry struct {
	channels map[string]Channel
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{channels: make(map[string]Channel)}
}

// Register adds a channel provider. Panics on duplicate registration to catch
// wiring errors at startup rather than at delivery time.
func (r *Registry) Register(ch Channel) {
	if _, exists := r.channels[ch.Type()]; exists {
		panic(fmt.Sprintf("channel: duplicate registration for type %q", ch.Type()))
	}
	r.channels[ch.Type()] = ch
}

// Get returns the Channel for the given type, or a NotFound error.
func (r *Registry) Get(channelType string) (Channel, error) {
	ch, ok := r.channels[channelType]
	if !ok {
		return nil, pkgerrors.NotFound("channel", channelType)
	}
	return ch, nil
}
