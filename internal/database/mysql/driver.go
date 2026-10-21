package mysql

import (
	"github.com/yazeed1s/quarro/internal/database/capability"
	"github.com/yazeed1s/quarro/internal/database/driver"
)

var (
	_ capability.SQLProvider          = Provider{}
	_ capability.Aggregator           = Provider{}
	_ capability.Explainer            = Provider{}
	_ capability.RoutineDiscoverer    = Provider{}
	_ capability.LazyDiscoverer       = Provider{}
	_ capability.ConstraintDiscoverer = Provider{}
	_ capability.DefinitionDiscoverer = Provider{}
)

func init() {
	driver.Register(Driver{})
}

// Driver registers MySQL with internal/database/driver.
type Driver struct{}

func (Driver) Metadata() driver.DriverMetadata {
	return driver.DriverMetadata{
		Name:        "mysql",
		DisplayName: "MySQL",
		Icon:        "mysql",
		SQLDialect:  "mysql",
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
				Default:  "3306",
				Required: true,
			},
			{
				Key:      "database",
				Label:    "Database",
				Kind:     "text",
				Required: true,
			},
			{
				Key:      "username",
				Label:    "Username",
				Kind:     "text",
				Required: true,
			},
			{Key: "password", Label: "Password", Kind: "password"},
		},
	}
}

// Capabilities sets RequiresDelimiterWrapping: a stored routine body sent
// for EXPLAIN or execution needs its own DELIMITER change first, since it
// contains semicolons that would otherwise end the statement early.
func (Driver) Capabilities() capability.Capabilities {
	c := capability.Of(New())
	c.SupportsAnalyze = true
	c.RequiresDelimiterWrapping = true
	c.SupportsJoins = true
	c.SupportsGroupBy = true
	return c
}

func (Driver) New() capability.Engine { return New() }
