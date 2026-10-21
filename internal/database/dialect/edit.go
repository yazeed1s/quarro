// Package dialect holds the SQL-generation and text-scanning algorithms
// shared by every database/sql-backed engine package (sqlite, postgres,
// mysql). Each algorithm here is parameterized by small dialect closures
// (quoting, placeholders, literal encoding) instead of an engine check, so
// adding another database/sql engine never means copying these algorithms.
package dialect

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// EditDialect supplies one engine's quoting, parameter, and NULL-comparison
// rules to ApplyEdits.
type EditDialect struct {
	Quote            func(string) string
	Param            func(int) string
	TableName        string
	DefaultValuesSQL string
	NullSafe         func(column, param string) string
}

// PreviewEdits builds the exact parameterized statements ApplyEdits would
// execute, without requiring a connection and without changing database
// state. Statement construction stays in buildEdit so preview and execution
// cannot acquire separate SQL-generation implementations.
func PreviewEdits(
	d EditDialect,
	table model.Table,
	edits []model.RowEdit,
) ([]model.EditPreview, error) {
	if len(edits) == 0 {
		return []model.EditPreview{}, nil
	}
	columns, keys, err := editColumns(table)
	if err != nil {
		return nil, err
	}
	out := make([]model.EditPreview, 0, len(edits))
	for _, edit := range edits {
		stmt, args, err := buildEdit(d, table, columns, keys, edit)
		if err != nil {
			return nil, err
		}
		out = append(out, model.EditPreview{
			Kind: edit.Kind,
			SQL:  stmt,
			Args: formatArgs(args),
		})
	}
	return out, nil
}

// ApplyEdits runs a batch of staged row edits against table through conn,
// building each statement with d's quoting/parameter rules. It requires a
// primary key (or other reliable identity) on table and stops at the first
// failure, including a stale-row conflict (an update/delete that affects a
// row count other than 1).
func ApplyEdits(
	ctx context.Context,
	conn *sql.Conn,
	d EditDialect,
	table model.Table,
	edits []model.RowEdit,
) (model.EditResult, error) {
	if len(edits) == 0 {
		return model.EditResult{}, nil
	}
	columns, keys, err := editColumns(table)
	if err != nil {
		return model.EditResult{}, err
	}
	result := model.EditResult{Statements: []string{}}
	for _, edit := range edits {
		stmt, args, err := buildEdit(d, table, columns, keys, edit)
		if err != nil {
			return result, err
		}
		res, err := conn.ExecContext(ctx, stmt, args...)
		if err != nil {
			return result, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return result, err
		}
		if edit.Kind != "insert" && affected != 1 {
			return result, fmt.Errorf(
				"conflict detected: expected to affect 1 row, affected %d; refresh before applying more edits",
				affected,
			)
		}
		result.Applied++
		result.RowsAffected += affected
		result.Statements = append(result.Statements, stmt)
	}
	return result, nil
}

func editColumns(table model.Table) (map[string]model.Column, []string, error) {
	if table.Kind != "table" {
		return nil, nil, fmt.Errorf(
			"only base tables can be edited; views stay read-only",
		)
	}
	columns := map[string]model.Column{}
	keys := []string{}
	for _, c := range table.Columns {
		columns[c.Name] = c
		if c.PrimaryKey {
			keys = append(keys, c.Name)
		}
	}
	if len(keys) == 0 {
		return nil, nil, fmt.Errorf(
			"editing requires a primary key or other reliable row identity",
		)
	}
	return columns, keys, nil
}

func formatArgs(args []any) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		switch value := arg.(type) {
		case nil:
			out = append(out, "NULL")
		case string:
			out = append(out, strconv.Quote(value))
		case []byte:
			out = append(out, fmt.Sprintf("<binary, %d bytes>", len(value)))
		case time.Time:
			out = append(out, strconv.Quote(value.Format(time.RFC3339Nano)))
		case json.Number:
			out = append(out, value.String())
		default:
			out = append(out, fmt.Sprint(value))
		}
	}
	return out
}

func buildEdit(
	d EditDialect,
	table model.Table,
	columns map[string]model.Column,
	keys []string,
	edit model.RowEdit,
) (string, []any, error) {
	if hasBlobPlaceholder(edit.Original) {
		return "", nil, fmt.Errorf(
			"rows containing BLOB placeholders cannot be edited safely yet",
		)
	}
	switch edit.Kind {
	case "insert":
		return buildInsert(d, columns, edit.Values)
	case "update":
		if len(edit.Values) == 0 {
			return "", nil, fmt.Errorf(
				"update has no changed columns",
			)
		}
		return buildUpdate(d, table, columns, keys, edit)
	case "delete":
		return buildDelete(d, table, columns, keys, edit)
	default:
		return "", nil, fmt.Errorf(
			"unsupported edit kind %q",
			edit.Kind,
		)
	}
}

func buildInsert(
	d EditDialect,
	columns map[string]model.Column,
	values map[string]model.CellEdit,
) (string, []any, error) {
	names := sortedKeys(values)
	args := []any{}
	cols := []string{}
	vals := []string{}
	for _, name := range names {
		edit := values[name]
		if _, ok := columns[name]; !ok {
			return "", nil, fmt.Errorf(
				"column %s no longer exists",
				name,
			)
		}
		if edit.Mode == "default" {
			continue
		}
		cols = append(cols, d.Quote(name))
		if edit.Mode == "null" {
			vals = append(vals, "NULL")
		} else if edit.Mode == "value" {
			args = append(args, edit.Value)
			vals = append(vals, d.Param(len(args)))
		} else {
			return "", nil, fmt.Errorf("unsupported cell edit mode %q", edit.Mode)
		}
	}
	if len(cols) == 0 {
		return "INSERT INTO " + d.TableName + d.DefaultValuesSQL, nil, nil
	}
	return "INSERT INTO " + d.TableName + " (" + strings.Join(
		cols,
		", ",
	) + ") VALUES (" + strings.Join(
		vals,
		", ",
	) + ")", args, nil
}

func buildUpdate(
	d EditDialect,
	table model.Table,
	columns map[string]model.Column,
	keys []string,
	edit model.RowEdit,
) (string, []any, error) {
	names := sortedKeys(edit.Values)
	args := []any{}
	set := []string{}
	for _, name := range names {
		cell := edit.Values[name]
		if _, ok := columns[name]; !ok {
			return "", nil, fmt.Errorf(
				"column %s no longer exists",
				name,
			)
		}
		if cell.Mode == "default" {
			return "", nil, fmt.Errorf(
				"default update is not supported yet; choose an explicit value or NULL",
			)
		}
		if cell.Mode == "null" {
			set = append(set, d.Quote(name)+" = NULL")
		} else if cell.Mode == "value" {
			args = append(args, cell.Value)
			set = append(set, d.Quote(name)+" = "+d.Param(len(args)))
		} else {
			return "", nil, fmt.Errorf("unsupported cell edit mode %q", cell.Mode)
		}
	}
	where, whereArgs, err := conflictWhere(
		d,
		table,
		columns,
		keys,
		edit.Original,
		len(args),
	)
	if err != nil {
		return "", nil, err
	}
	args = append(args, whereArgs...)
	return "UPDATE " + d.TableName + " SET " + strings.Join(
		set,
		", ",
	) + " WHERE " + where, args, nil
}

func buildDelete(
	d EditDialect,
	table model.Table,
	columns map[string]model.Column,
	keys []string,
	edit model.RowEdit,
) (string, []any, error) {
	where, args, err := conflictWhere(
		d,
		table,
		columns,
		keys,
		edit.Original,
		0,
	)
	if err != nil {
		return "", nil, err
	}
	return "DELETE FROM " + d.TableName + " WHERE " + where, args, nil
}

func conflictWhere(
	d EditDialect,
	table model.Table,
	columns map[string]model.Column,
	keys []string,
	original map[string]any,
	offset int,
) (string, []any, error) {
	for _, key := range keys {
		if _, ok := original[key]; !ok {
			return "", nil, fmt.Errorf(
				"primary-key column %s is missing from the loaded row",
				key,
			)
		}
	}
	parts := []string{}
	args := []any{}
	for _, col := range table.Columns {
		if _, ok := columns[col.Name]; !ok {
			continue
		}
		value, ok := original[col.Name]
		if !ok {
			return "", nil, fmt.Errorf(
				"loaded row is missing column %s; refresh before editing",
				col.Name,
			)
		}
		if isBlobPlaceholder(value) {
			return "", nil, fmt.Errorf(
				"rows containing BLOB placeholders cannot be edited safely yet",
			)
		}
		args = append(args, value)
		parts = append(
			parts,
			d.NullSafe(d.Quote(col.Name), d.Param(offset+len(args))),
		)
	}
	return strings.Join(parts, " AND "), args, nil
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func hasBlobPlaceholder(row map[string]any) bool {
	for _, v := range row {
		if isBlobPlaceholder(v) {
			return true
		}
	}
	return false
}

func isBlobPlaceholder(v any) bool {
	switch x := v.(type) {
	case model.BlobValue:
		return x.Blob
	case map[string]any:
		b, _ := x["blob"].(bool)
		return b
	}
	return false
}
