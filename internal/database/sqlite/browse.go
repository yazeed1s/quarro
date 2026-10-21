package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Browse builds SQLite-specific SQL here, binding filter values separately.
// filterColumn, when non-empty, restricts filter to an exact match on that
// one column instead of the default free-text substring search across
// every column.
func Browse(
	ctx context.Context,
	conn *sql.Conn,
	table, filter, filterColumn, sort string,
	descending bool,
	offset, pageSize int,
) ResultSet {
	if offset < 0 || offset > 1000000 {
		return ResultSet{
			Kind:  "error",
			Error: "Page offset is out of range.",
		}
	}
	if pageSize < 1 || pageSize > 10000 {
		pageSize = 500
	}
	var kind string
	if err := conn.QueryRowContext(ctx, `SELECT type FROM sqlite_schema WHERE name = ? AND type IN ('table','view')`, table).Scan(&kind); err != nil {
		return ResultSet{
			Kind:  "error",
			Error: "Table or view no longer exists.",
		}
	}
	rows, err := conn.QueryContext(
		ctx,
		`SELECT name, pk FROM pragma_table_xinfo(?)`,
		table,
	)
	if err != nil {
		return ResultSet{Kind: "error", Error: err.Error()}
	}
	names := []string{}
	keys := []string{}
	for rows.Next() {
		var name string
		var pk int
		if err := rows.Scan(&name, &pk); err != nil {
			rows.Close()
			return ResultSet{Kind: "error", Error: err.Error()}
		}
		names = append(names, name)
		if pk > 0 {
			keys = append(keys, name)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return ResultSet{Kind: "error", Error: err.Error()}
	}
	contains := func(name string) bool {
		for _, n := range names {
			if n == name {
				return true
			}
		}
		return false
	}
	if sort != "" && !contains(sort) {
		return ResultSet{
			Kind:  "error",
			Error: "Sort column no longer exists.",
		}
	}
	if filterColumn != "" && !contains(filterColumn) {
		return ResultSet{
			Kind:  "error",
			Error: "Filter column no longer exists.",
		}
	}
	sqlText := `SELECT * FROM ` + quoteIdent(table)
	args := []any{}
	if filterColumn != "" {
		sqlText += ` WHERE CAST(` + quoteIdent(filterColumn) + ` AS TEXT) = ?`
		args = append(args, filter)
	} else if filter != "" {
		parts := []string{}
		for _, name := range names {
			parts = append(
				parts,
				`instr(lower(CAST(`+quoteIdent(
					name,
				)+` AS TEXT)), lower(?)) > 0`,
			)
			args = append(args, filter)
		}
		if len(parts) > 0 {
			sqlText += ` WHERE (` + strings.Join(parts, ` OR `) + `)`
		}
	}
	order := []string{}
	if sort != "" {
		direction := " ASC"
		if descending {
			direction = " DESC"
		}
		order = append(order, quoteIdent(sort)+direction)
	}
	for _, key := range keys {
		if key != sort {
			order = append(order, quoteIdent(key))
		}
	}
	stable := len(keys) > 0
	if kind == "table" {
		// A non-shadowed rowid is an unambiguous tie-breaker, including nullable
		// primary keys on ordinary SQLite tables. WITHOUT ROWID tables use PKs.
		for _, alias := range []string{"_rowid_", "rowid", "oid"} {
			if contains(alias) {
				continue
			}
			probe, e := conn.QueryContext(
				ctx,
				`SELECT `+alias+` FROM `+quoteIdent(table)+` LIMIT 0`,
			)
			if e == nil {
				probe.Close()
				order = append(order, alias)
				stable = true
			}
			break
		}
	}
	if len(order) > 0 {
		sqlText += ` ORDER BY ` + strings.Join(order, `, `)
	}
	sqlText += ` LIMIT ? OFFSET ?`
	args = append(args, pageSize+1, offset)
	result := ExecuteOnConn(
		ctx,
		conn,
		sqlText,
		pageSize,
		MaxResultBytes,
		args...)
	result.StableOrder = stable
	// Statement is a readable plan, not a query containing unescaped values.
	result.Statement = fmt.Sprintf(
		"%s /* page offset %d */",
		sqlText,
		offset,
	)
	return result
}
