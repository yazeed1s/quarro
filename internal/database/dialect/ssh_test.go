package dialect

import (
	"testing"

	"github.com/yazeed1s/quarro/internal/connections"
)

func TestValidateSSHProfile(t *testing.T) {
	base := connections.Profile{
		Engine:      connections.EnginePostgres,
		SSHEnabled:  true,
		SSHHost:     "jump.example.com",
		SSHUsername: "deploy",
	}
	cases := []struct {
		name    string
		modify  func(p connections.Profile) connections.Profile
		wantErr bool
	}{
		{
			name:    "disabled is always valid",
			modify:  func(p connections.Profile) connections.Profile { p.SSHEnabled = false; p.Engine = ""; return p },
			wantErr: false,
		},
		{
			name: "password method is valid",
			modify: func(p connections.Profile) connections.Profile {
				p.SSHAuthMethod = "password"
				return p
			},
			wantErr: false,
		},
		{
			name: "key method with a path is valid",
			modify: func(p connections.Profile) connections.Profile {
				p.SSHAuthMethod = "key"
				p.SSHPrivateKeyPath = "/home/u/.ssh/id_ed25519"
				return p
			},
			wantErr: false,
		},
		{
			name: "mysql is allowed",
			modify: func(p connections.Profile) connections.Profile {
				p.Engine = connections.EngineMySQL
				p.SSHAuthMethod = "password"
				return p
			},
			wantErr: false,
		},
		{
			name: "sqlite is rejected",
			modify: func(p connections.Profile) connections.Profile {
				p.Engine = connections.EngineSQLite
				p.SSHAuthMethod = "password"
				return p
			},
			wantErr: true,
		},
		{
			name: "mongodb is rejected",
			modify: func(p connections.Profile) connections.Profile {
				p.Engine = connections.EngineMongoDB
				p.SSHAuthMethod = "password"
				return p
			},
			wantErr: true,
		},
		{
			name: "unix socket is rejected",
			modify: func(p connections.Profile) connections.Profile {
				p.SSHAuthMethod = "password"
				p.Socket = "/tmp/pg.sock"
				return p
			},
			wantErr: true,
		},
		{
			name: "missing host is rejected",
			modify: func(p connections.Profile) connections.Profile {
				p.SSHAuthMethod = "password"
				p.SSHHost = ""
				return p
			},
			wantErr: true,
		},
		{
			name: "missing username is rejected",
			modify: func(p connections.Profile) connections.Profile {
				p.SSHAuthMethod = "password"
				p.SSHUsername = ""
				return p
			},
			wantErr: true,
		},
		{
			name: "key method without a path is rejected",
			modify: func(p connections.Profile) connections.Profile {
				p.SSHAuthMethod = "key"
				return p
			},
			wantErr: true,
		},
		{
			name: "unknown auth method is rejected",
			modify: func(p connections.Profile) connections.Profile {
				p.SSHAuthMethod = "agent"
				return p
			},
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateSSHProfile(c.modify(base))
			if c.wantErr && err == nil {
				t.Fatal("ValidateSSHProfile() = nil, want an error")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("ValidateSSHProfile() = %v, want nil", err)
			}
		})
	}
}
