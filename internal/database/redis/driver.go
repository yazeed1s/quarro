package redis

import (
	"github.com/yazeed1s/quarro/internal/database/capability"
	"github.com/yazeed1s/quarro/internal/database/driver"
)

// Compile-time checks that Provider satisfies exactly the capabilities it
// claims (see this package's doc comment): Connector, Discoverer,
// KeyValueStore, KeyValueEditor, NativeQueryExecutor (redis-cli style
// commands), and NativeWriteDetector, never Browser or any SQL-shaped
// capability.
var (
	_ capability.Connector      = Provider{}
	_ capability.Discoverer     = Provider{}
	_ capability.KeyValueStore  = Provider{}
	_ capability.KeyValueEditor = Provider{}

	_ capability.NativeQueryExecutor = Provider{}
	_ capability.NativeWriteDetector = Provider{}
)

func init() {
	driver.Register(Driver{})
}

// Driver registers Redis with internal/database/driver.
type Driver struct{}

func (Driver) Metadata() driver.DriverMetadata {
	return driver.DriverMetadata{
		Name:        "redis",
		DisplayName: "Redis",
		Icon:        "redis",
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
				Default:  "6379",
				Required: true,
			},
			{Key: "password", Label: "Password", Kind: "password"},
		},
	}
}

// Capabilities leaves SupportsAnalyze/SupportsJoins/SupportsGroupBy false:
// Redis implements neither Explainer nor Aggregator. Its native queries
// are redis-cli style command lines.
func (Driver) Capabilities() capability.Capabilities {
	c := capability.Of(New())
	c.NativeQueryLanguage = "redis-command"
	return c
}

func (Driver) New() capability.Engine { return New() }
