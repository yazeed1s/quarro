package driver

import "fmt"

var (
	order  []string
	byName = map[string]Driver{}
)

// Register adds d to the registry, keyed by its own metadata name. Engine
// packages call this from their own init(), so this package never imports
// a concrete engine package. Re-registering the same name replaces the
// prior entry without changing its position in List's order.
func Register(d Driver) {
	name := d.Metadata().Name
	if _, exists := byName[name]; !exists {
		order = append(order, name)
	}
	byName[name] = d
}

// Get looks up a registered driver by name.
func Get(name string) (Driver, error) {
	d, ok := byName[name]
	if !ok {
		return nil, fmt.Errorf("unsupported database engine %q", name)
	}
	return d, nil
}

// List returns every registered driver's metadata, in registration order.
func List() []DriverMetadata {
	list := make([]DriverMetadata, 0, len(order))
	for _, name := range order {
		list = append(list, byName[name].Metadata())
	}
	return list
}
