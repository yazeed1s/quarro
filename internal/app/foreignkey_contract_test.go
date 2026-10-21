package app

import (
	"os"
	"strings"
	"testing"

	"github.com/yazeed1s/quarro/internal/database/provider"
	"github.com/yazeed1s/quarro/internal/jobs"
)

// TestForeignKeyReferentialActions confirms PostgreSQL and MySQL both report
// a foreign key's real ON DELETE/ON UPDATE action through DiscoverSchema,
// including the engine's own default ("NO ACTION") when a FOREIGN KEY
// clause specifies none explicitly.
func TestForeignKeyReferentialActions(t *testing.T) {
	for _, engine := range []string{"postgres", "mysql"} {
		engine := engine
		t.Run(engine, func(t *testing.T) {
			url := os.Getenv("QUARRO_TEST_" + strings.ToUpper(engine) + "_URL")
			if url == "" {
				t.Skipf("requires an isolated %s URL", engine)
			}
			svc := newTestService(t)
			defer svc.sessions.CloseAll()
			queries := NewQueriesService(svc.sessions, jobs.NewRegistry())

			p, e := provider.ImportURL(url)
			if e != nil {
				t.Fatal(e)
			}
			p.ID = engine
			p.Name = engine
			if _, e := svc.SaveProfile(p); e != nil {
				t.Fatal(e)
			}
			if result, e := svc.Connect(p.ID, p.ID); e != nil || !result.OK {
				t.Fatalf("connect: %+v %v", result, e)
			}

			ddl := `CREATE TABLE fk_contract_users (id INT PRIMARY KEY);
INSERT INTO fk_contract_users (id) VALUES (1),(10),(21);
CREATE TABLE fk_contract_orders (
    id INT PRIMARY KEY,
    user_id INT,
    FOREIGN KEY (user_id) REFERENCES fk_contract_users(id) ON DELETE CASCADE ON UPDATE SET NULL
);
CREATE TABLE fk_contract_line_items (
    id INT PRIMARY KEY,
    order_id INT,
    FOREIGN KEY (order_id) REFERENCES fk_contract_orders(id)
);`
			if result, e := queries.ExecuteDocument(p.ID, "fk-contract-doc", "fk-contract-job", ddl, 10, 10); e != nil {
				t.Fatalf("ddl: %v", e)
			} else {
				for _, r := range result.Results {
					if r.Kind == "error" {
						t.Fatalf("ddl statement failed: %s", r.Error)
					}
				}
			}

			tables, e := queries.DiscoverSchema(p.ID)
			if e != nil {
				t.Fatalf("DiscoverSchema: %v", e)
			}
			findFK := func(table, column string) (fk fkT, found bool) {
				for _, tbl := range tables {
					if tbl.ObjectName != table {
						continue
					}
					for _, f := range tbl.ForeignKeys {
						if f.Column == column {
							return fkT{onDelete: f.OnDelete, onUpdate: f.OnUpdate}, true
						}
					}
				}
				return fkT{}, false
			}
			explicit, ok := findFK("fk_contract_orders", "user_id")
			if !ok {
				t.Fatalf("fk_contract_orders.user_id FK not found in %+v", tables)
			}
			if explicit.onDelete != "CASCADE" || explicit.onUpdate != "SET NULL" {
				t.Errorf("%s explicit FK actions = %+v, want CASCADE/SET NULL", engine, explicit)
			}
			implicit, ok := findFK("fk_contract_line_items", "order_id")
			if !ok {
				t.Fatalf("fk_contract_line_items.order_id FK not found in %+v", tables)
			}
			if implicit.onDelete != "NO ACTION" || implicit.onUpdate != "NO ACTION" {
				t.Errorf("%s default FK actions = %+v, want NO ACTION/NO ACTION", engine, implicit)
			}

			// "Open related rows" relies on BrowseTable's filterColumn being an
			// exact match, not the default substring search that would also
			// match id 10 and 21 against a filter of "1".
			var usersTableID string
			for _, tbl := range tables {
				if tbl.ObjectName == "fk_contract_users" {
					usersTableID = tbl.Name
				}
			}
			if usersTableID == "" {
				t.Fatalf("%s fk_contract_users table id not found in %+v", engine, tables)
			}
			related, e := queries.BrowseTable(p.ID, "fk-related-doc", "fk-related-job", usersTableID, "1", "id", "", false, 0, 10, 10)
			if e != nil || related.Results[0].Kind != "rows" {
				t.Fatalf("%s BrowseTable with filterColumn: %+v %v", engine, related.Results[0], e)
			}
			rows := related.Results[0].Rows
			if len(rows) != 1 {
				t.Fatalf("%s exact filter on id=1 matched %d rows, want 1: %+v", engine, len(rows), rows)
			}
		})
	}
}

type fkT struct {
	onDelete, onUpdate string
}
