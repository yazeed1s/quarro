package sqlite

import (
	"context"
	"testing"
)

func TestDiscoverTablesViewsColumnsIndexesForeignKeys(t *testing.T) {
	db := openTestDB(t)
	ddl := []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)`,
		`CREATE UNIQUE INDEX idx_users_email ON users(email)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE ON UPDATE SET NULL)`,
		`CREATE TABLE line_items (id INTEGER PRIMARY KEY, order_id INTEGER, FOREIGN KEY(order_id) REFERENCES orders(id))`,
		`CREATE VIEW active_users AS SELECT * FROM users WHERE 1=1`,
	}
	for _, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}

	tables, err := Discover(db)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(tables) != 4 {
		t.Fatalf(
			"got %d tables/views, want 4: %+v",
			len(tables),
			tables,
		)
	}

	byName := map[string]Table{}
	for _, tbl := range tables {
		byName[tbl.Name] = tbl
	}

	users, ok := byName["users"]
	if !ok || users.Kind != "table" {
		t.Fatalf("users = %+v", users)
	}
	if len(users.Columns) != 3 {
		t.Fatalf("users columns = %+v", users.Columns)
	}
	if !users.Columns[0].PrimaryKey {
		t.Errorf(
			"users.id should be primary key: %+v",
			users.Columns[0],
		)
	}
	if !users.Columns[1].NotNull {
		t.Errorf(
			"users.name should be NOT NULL: %+v",
			users.Columns[1],
		)
	}
	if len(users.Indexes) != 1 || !users.Indexes[0].Unique {
		t.Fatalf("users indexes = %+v", users.Indexes)
	}

	orders, ok := byName["orders"]
	if !ok {
		t.Fatal("orders table missing")
	}
	if len(orders.ForeignKeys) != 1 {
		t.Fatalf("orders foreign keys = %+v", orders.ForeignKeys)
	}
	fk := orders.ForeignKeys[0]
	if fk.Column != "user_id" || fk.RefTable != "users" ||
		fk.RefColumn != "id" {
		t.Errorf("orders FK = %+v", fk)
	}
	if fk.OnDelete != "CASCADE" || fk.OnUpdate != "SET NULL" {
		t.Errorf("orders FK referential actions = %+v", fk)
	}

	lineItems, ok := byName["line_items"]
	if !ok || len(lineItems.ForeignKeys) != 1 {
		t.Fatalf("line_items foreign keys = %+v", lineItems.ForeignKeys)
	}
	if got := lineItems.ForeignKeys[0]; got.OnDelete != "NO ACTION" || got.OnUpdate != "NO ACTION" {
		t.Errorf("line_items FK with no explicit action should default to NO ACTION: %+v", got)
	}

	view, ok := byName["active_users"]
	if !ok || view.Kind != "view" {
		t.Fatalf("active_users = %+v", view)
	}
	if len(view.Columns) != 3 {
		t.Fatalf("active_users columns = %+v", view.Columns)
	}
}

func TestDiscoverEmptyDatabase(t *testing.T) {
	db := openTestDB(t)
	tables, err := Discover(db)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(tables) != 0 {
		t.Fatalf("got %+v, want none", tables)
	}
}

func TestDiscoverDefinitionIsVerbatim(t *testing.T) {
	db := openTestDB(t)
	tableSQL := `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`
	viewSQL := `CREATE VIEW active_users AS SELECT * FROM users WHERE 1=1`
	for _, stmt := range []string{tableSQL, viewSQL} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	p := Provider{}
	tableDef, err := p.DiscoverDefinition(context.Background(), db, "", "users")
	if err != nil {
		t.Fatal(err)
	}
	if tableDef.SQL != tableSQL || !tableDef.Verbatim {
		t.Errorf("table definition = %+v, want verbatim %q", tableDef, tableSQL)
	}
	viewDef, err := p.DiscoverDefinition(context.Background(), db, "", "active_users")
	if err != nil {
		t.Fatal(err)
	}
	if viewDef.SQL != viewSQL || !viewDef.Verbatim {
		t.Errorf("view definition = %+v, want verbatim %q", viewDef, viewSQL)
	}
	if _, err := p.DiscoverDefinition(context.Background(), db, "", "missing"); err == nil {
		t.Error("expected an error for a table that does not exist")
	}
}

func TestQuoteIdentEscapesQuotes(t *testing.T) {
	if got := quoteIdent(`weird"name`); got != `"weird""name"` {
		t.Errorf("quoteIdent = %q", got)
	}
}
