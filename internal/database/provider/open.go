package provider

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/mongodb"
	"github.com/yazeed1s/quarro/internal/database/redis"
)

// Validate checks a connection profile's fields, including (for a remote
// engine) that the engine itself is registered. MongoDB and Redis get their
// own validation rather than dialect.ValidateProfile: unlike every SQL
// engine here, neither requires a username: an authless local/dev instance
// is common for both.
func Validate(p connections.Profile) error {
	if p.ID == "" || strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("connection name and ID are required")
	}
	if p.Engine != connections.EngineSQLite {
		if _, e := Get(p.Engine); e != nil {
			return e
		}
	}
	if e := dialect.ValidateSSHProfile(p); e != nil {
		return e
	}
	switch p.Engine {
	case connections.EngineMongoDB:
		return mongodb.ValidateProfile(p)
	case connections.EngineRedis:
		return redis.ValidateProfile(p)
	}
	return dialect.ValidateProfile(p)
}

// ImportURL accepts a small explicit option set; it never persists the URL.
func ImportURL(raw string) (connections.Input, error) {
	u, e := url.Parse(raw)
	if e != nil {
		return connections.Input{}, fmt.Errorf(
			"invalid connection URL",
		)
	}
	p := connections.Input{}
	switch u.Scheme {
	case "postgres", "postgresql":
		p.Engine = connections.EnginePostgres
		p.Schema = "public"
		p.Port = "5432"
	case "mysql":
		p.Engine = connections.EngineMySQL
		p.Port = "3306"
	default:
		return p, fmt.Errorf("use a postgres:// or mysql:// URL")
	}
	if u.Fragment != "" {
		return p, fmt.Errorf("URL fragments are not supported")
	}
	p.Host = u.Hostname()
	if u.Port() != "" {
		p.Port = u.Port()
	}
	p.Database = strings.TrimPrefix(u.Path, "/")
	if u.User != nil {
		p.Username = u.User.Username()
		p.Password, _ = u.User.Password()
	}
	p.TLSMode = "verify-full"
	query, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return p, fmt.Errorf("invalid URL options")
	}
	for k, v := range query {
		if len(v) != 1 {
			return p, fmt.Errorf("duplicate URL option %s", k)
		}
		switch k {
		case "sslmode":
			p.TLSMode = v[0]
		case "sslrootcert":
			p.TLSCA = v[0]
		case "sslcert":
			p.TLSCert = v[0]
		case "sslkey":
			p.TLSKey = v[0]
		case "serverName":
			p.TLSServerName = v[0]
		case "socket":
			p.Socket = v[0]
		case "schema":
			p.Schema = v[0]
		default:
			return p, fmt.Errorf("unsupported URL option %s", k)
		}
	}
	p.ID = "import"
	p.Name = p.Database
	if e := Validate(p.Profile); e != nil {
		return connections.Input{}, e
	}
	return p, nil
}
