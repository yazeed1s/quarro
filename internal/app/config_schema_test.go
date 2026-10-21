package app

import (
	"testing"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/capability"
	"github.com/yazeed1s/quarro/internal/database/driver"
)

// fakePluginDriver stands in for a hypothetical sixth engine whose
// connection needs a field (an API key) that connections.Profile has no
// named struct field for, proving a driver's ConfigSchema can require a
// Profile.Config key without a change to the Profile struct.
type fakePluginDriver struct{}

func (fakePluginDriver) Metadata() driver.DriverMetadata {
	return driver.DriverMetadata{
		Name:        "fakeplugin",
		DisplayName: "Fake Plugin",
		ConfigSchema: []driver.ConfigField{
			{
				Key:      "apiKey",
				Label:    "API key",
				Kind:     "password",
				Required: true,
			},
		},
	}
}

func (fakePluginDriver) Capabilities() capability.Capabilities { return capability.Capabilities{} }

func (fakePluginDriver) New() capability.Engine { return nil }

func TestValidateConfigSchemaRequiresUnnamedField(t *testing.T) {
	driver.Register(fakePluginDriver{})
	p := connections.Profile{ID: "x", Name: "x", Engine: "fakeplugin"}
	if err := validateConfigSchema(p); err == nil {
		t.Fatal("expected missing apiKey to fail validation")
	}
	p.Config = map[string]string{"apiKey": "secret"}
	if err := validateConfigSchema(p); err != nil {
		t.Fatalf("unexpected error with apiKey present: %v", err)
	}
}

func TestValidateConfigSchemaBuiltinsNeedNoConfig(t *testing.T) {
	for _, e := range []connections.Engine{
		connections.EngineSQLite, connections.EnginePostgres, connections.EngineMySQL,
		connections.EngineMongoDB, connections.EngineRedis,
	} {
		p := connections.Profile{ID: "x", Name: "x", Engine: e}
		if err := validateConfigSchema(p); err != nil {
			t.Fatalf(
				"%s: unexpected error with no Config: %v",
				e,
				err,
			)
		}
	}
}
