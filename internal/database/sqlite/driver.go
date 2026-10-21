package sqlite

import (
	"github.com/yazeed1s/quarro/internal/database/capability"
	"github.com/yazeed1s/quarro/internal/database/driver"
)

// Compile-time checks that Provider satisfies the full SQL surface plus
// the capabilities every SQL engine here also implements.
var (
	_ capability.SQLProvider          = Provider{}
	_ capability.Aggregator           = Provider{}
	_ capability.Explainer            = Provider{}
	_ capability.DefinitionDiscoverer = Provider{}
)

func init() {
	driver.Register(Driver{})
}

// Driver registers SQLite with internal/database/driver.
type Driver struct{}

func (Driver) Metadata() driver.DriverMetadata {
	return driver.DriverMetadata{
		Name:        "sqlite",
		DisplayName: "SQLite",
		Icon:        "sqlite",
		SQLDialect:  "sqlite",
		ConfigSchema: []driver.ConfigField{
			{
				Key:      "path",
				Label:    "File",
				Kind:     "filepath",
				Required: true,
			},
			{Key: "readOnly", Label: "Read-only", Kind: "bool"},
		},
	}
}

// Capabilities leaves SupportsAnalyze false: SQLite's EXPLAIN has no
// execution-analysis mode, only a static plan estimate.
func (Driver) Capabilities() capability.Capabilities {
	c := capability.Of(New())
	c.SupportsJoins = true
	c.SupportsGroupBy = true
	return c
}

func (Driver) New() capability.Engine { return New() }
