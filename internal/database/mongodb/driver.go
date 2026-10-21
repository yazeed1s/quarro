package mongodb

import (
	"github.com/yazeed1s/quarro/internal/database/capability"
	"github.com/yazeed1s/quarro/internal/database/driver"
)

// Compile-time checks that Provider satisfies exactly the capabilities it
// claims (see this package's doc comment), no more: never SQLExecutor,
// Editor, Transactor, QueryCompiler, or IdentifierFormatter.
var (
	_ capability.Connector           = Provider{}
	_ capability.Discoverer          = Provider{}
	_ capability.Browser             = Provider{}
	_ capability.Aggregator          = Provider{}
	_ capability.Explainer           = Provider{}
	_ capability.DocumentEditor      = Provider{}
	_ capability.NativeQueryExecutor = Provider{}
)

func init() {
	driver.Register(Driver{})
}

// Driver registers MongoDB with internal/database/driver.
type Driver struct{}

func (Driver) Metadata() driver.DriverMetadata {
	return driver.DriverMetadata{
		Name:        "mongodb",
		DisplayName: "MongoDB",
		Icon:        "mongodb",
		ConfigSchema: []driver.ConfigField{
			{
				Key:      "host",
				Label:    "Host",
				Kind:     "text",
				Required: true,
			},
			{
				Key:      "port",
				Label:    "Port",
				Kind:     "port",
				Default:  "27017",
				Required: true,
			},
			{
				Key:      "database",
				Label:    "Database",
				Kind:     "text",
				Required: true,
			},
			{Key: "username", Label: "Username", Kind: "text"},
			{Key: "password", Label: "Password", Kind: "password"},
		},
	}
}

// Capabilities leaves SupportsJoins/SupportsGroupBy false: Aggregate
// rejects a VisualQuery with joins rather than faking one with $lookup
// (see this package's doc comment). SupportsAnalyze is true: Explain's
// analyze flag runs a real "executionStats" verbosity, not just an
// estimate. Its native queries are aggregation pipelines.
func (Driver) Capabilities() capability.Capabilities {
	c := capability.Of(New())
	c.SupportsAnalyze = true
	c.NativeQueryLanguage = "mongodb-pipeline"
	return c
}

func (Driver) New() capability.Engine { return New() }
