package app

import (
	"fmt"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/driver"
)

// namedProfileFields are connections.Profile's own struct-backed keys.
// provider.Validate (dialect.ValidateProfile, mongodb.ValidateProfile,
// redis.ValidateProfile) already checks these; validateConfigSchema only
// checks a driver's remaining ConfigSchema keys, the ones a driver with a
// field Profile has no struct field for stores in Profile.Config instead.
var namedProfileFields = map[string]bool{
	"host": true, "port": true, "database": true, "username": true,
	"path": true, "readOnly": true, "socket": true,
	"tlsMode": true, "tlsCA": true, "tlsCert": true, "tlsKey": true, "tlsServerName": true,
}

// validateConfigSchema checks that a required ConfigSchema field p's
// driver declares, and connections.Profile has no named field for, is
// present in p.Config. It never rejects one of the five built-in engines
// today: every field their own ConfigSchema declares is in
// namedProfileFields, already checked by provider.Validate.
func validateConfigSchema(p connections.Profile) error {
	d, e := driver.Get(string(p.Engine))
	if e != nil {
		return e
	}
	for _, f := range d.Metadata().ConfigSchema {
		if !f.Required || namedProfileFields[f.Key] {
			continue
		}
		if p.Config[f.Key] == "" {
			return fmt.Errorf("%s is required", f.Label)
		}
	}
	return nil
}
