package registry

import (
	"fmt"
	"sort"

	"github.com/danni2019/starSling/internal/strategy"
)

type Registry struct {
	factories map[string]strategy.Factory
}

func New() *Registry {
	return &Registry{factories: map[string]strategy.Factory{}}
}

func (r *Registry) Register(name string, factory strategy.Factory) error {
	if name == "" {
		return fmt.Errorf("strategy name required")
	}
	if factory == nil {
		return fmt.Errorf("strategy factory required")
	}
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("strategy already registered: %s", name)
	}
	r.factories[name] = factory
	return nil
}

func (r *Registry) Create(name string) (strategy.Strategy, error) {
	factory, exists := r.factories[name]
	if !exists {
		return nil, fmt.Errorf("unknown strategy: %s", name)
	}
	return factory(), nil
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
