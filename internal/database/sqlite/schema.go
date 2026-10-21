package sqlite

import (
	"database/sql"

	"github.com/yazeed1s/quarro/internal/database/model"
)

type Column = model.Column
type Index = model.Index
type ForeignKey = model.ForeignKey
type Table = model.Table

// Discover lists tables and views with their columns, indexes, and foreign
// keys, via SQLite's schema pragmas.
func Discover(db *sql.DB) ([]Table, error) {
	rows, err := db.Query(`
		SELECT name, type FROM sqlite_master
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	type nameKind struct{ name, kind string }
	var items []nameKind
	for rows.Next() {
		var nk nameKind
		if err := rows.Scan(&nk.name, &nk.kind); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, nk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	tables := make([]Table, 0, len(items))
	for _, item := range items {
		columns, err := columnsOf(db, item.name)
		if err != nil {
			return nil, err
		}
		var indexes []Index
		var foreignKeys []ForeignKey
		if item.kind == "table" {
			indexes, err = indexesOf(db, item.name)
			if err != nil {
				return nil, err
			}
			foreignKeys, err = foreignKeysOf(db, item.name)
			if err != nil {
				return nil, err
			}
		}
		tables = append(tables, Table{
			Name:        item.name,
			Kind:        item.kind,
			Columns:     columns,
			Indexes:     indexes,
			ForeignKeys: foreignKeys,
		})
	}
	return tables, nil
}

func columnsOf(db *sql.DB, table string) ([]Column, error) {
	rows, err := db.Query(
		`PRAGMA table_xinfo(` + quoteIdent(table) + `)`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var columns []Column
	for rows.Next() {
		var (
			cid        int
			name       string
			ctype      string
			notNull    int
			defaultVal sql.NullString
			pk         int
			hidden     int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultVal, &pk, &hidden); err != nil {
			return nil, err
		}
		col := Column{
			Name:       name,
			Type:       ctype,
			NotNull:    notNull != 0,
			PrimaryKey: pk != 0,
		}
		if defaultVal.Valid {
			col.DefaultValue = &defaultVal.String
		}
		columns = append(columns, col)
	}
	return columns, rows.Err()
}

func indexesOf(db *sql.DB, table string) ([]Index, error) {
	rows, err := db.Query(
		`PRAGMA index_list(` + quoteIdent(table) + `)`,
	)
	if err != nil {
		return nil, err
	}
	type listRow struct {
		name   string
		unique bool
	}
	var list []listRow
	for rows.Next() {
		var (
			seq     int
			name    string
			unique  int
			origin  string
			partial int
		)
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, listRow{name: name, unique: unique != 0})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	indexes := make([]Index, 0, len(list))
	for _, idx := range list {
		infoRows, err := db.Query(
			`PRAGMA index_info(` + quoteIdent(idx.name) + `)`,
		)
		if err != nil {
			return nil, err
		}
		var columns []string
		for infoRows.Next() {
			var seqno, cid int
			var name sql.NullString
			if err := infoRows.Scan(&seqno, &cid, &name); err != nil {
				infoRows.Close()
				return nil, err
			}
			if name.Valid {
				columns = append(columns, name.String)
			}
		}
		if err := infoRows.Err(); err != nil {
			infoRows.Close()
			return nil, err
		}
		infoRows.Close()
		indexes = append(
			indexes,
			Index{
				Name:    idx.name,
				Columns: columns,
				Unique:  idx.unique,
			},
		)
	}
	return indexes, nil
}

func foreignKeysOf(db *sql.DB, table string) ([]ForeignKey, error) {
	rows, err := db.Query(
		`PRAGMA foreign_key_list(` + quoteIdent(table) + `)`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var fks []ForeignKey
	for rows.Next() {
		var (
			id, seq                     int
			refTable, from              string
			to                          sql.NullString
			onUpdate, onDelete, matchOn string
		)
		if err := rows.Scan(&id, &seq, &refTable, &from, &to, &onUpdate, &onDelete, &matchOn); err != nil {
			return nil, err
		}
		fks = append(
			fks,
			ForeignKey{
				Column:    from,
				RefTable:  refTable,
				RefColumn: to.String,
				OnDelete:  onDelete,
				OnUpdate:  onUpdate,
			},
		)
	}
	return fks, rows.Err()
}

// quoteIdent double-quotes a SQL identifier, escaping embedded quotes, for
// safe interpolation into PRAGMA statements (which don't accept bound
// parameters for object names).
func quoteIdent(name string) string {
	out := make([]byte, 0, len(name)+2)
	out = append(out, '"')
	for i := 0; i < len(name); i++ {
		if name[i] == '"' {
			out = append(out, '"', '"')
		} else {
			out = append(out, name[i])
		}
	}
	out = append(out, '"')
	return string(out)
}
