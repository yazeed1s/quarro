package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/yazeed1s/quarro/internal/database/model"
	modernsqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// maxSafeInt is JavaScript's Number.MAX_SAFE_INTEGER. Go int64 values beyond
// this (in either direction) are marshaled as decimal strings instead of
// JSON numbers, so large integers never lose precision passing through JS.
const maxSafeInt = 1<<53 - 1

type ColumnMeta = model.ColumnMeta
type BlobValue = model.BlobValue
type ResultSet = model.ResultSet

const MaxResultBytes = 4 << 20

// ExecuteOne is the standalone helper used by provider tests.
func ExecuteOne(
	ctx context.Context,
	db *sql.DB,
	stmt string,
	maxRows int,
) ResultSet {
	conn, err := db.Conn(ctx)
	if err != nil {
		return ResultSet{
			Kind:      "error",
			Statement: stmt,
			Error:     err.Error(),
		}
	}
	defer conn.Close()
	return ExecuteOnConn(ctx, conn, stmt, maxRows, MaxResultBytes)
}

func ExecuteOnConn(
	ctx context.Context,
	db *sql.Conn,
	stmt string,
	maxRows, maxBytes int,
	args ...any,
) ResultSet {
	if maxRows < 1 || maxRows > 10000 {
		maxRows = 500
	}
	// Bound individual SQLite values before the driver allocates/encodes them.
	if _, err := modernsqlite.Limit(db, sqlite3.SQLITE_LIMIT_LENGTH, 8<<20); err != nil {
		return ResultSet{Kind: "error", Error: err.Error()}
	}
	var columnCount int
	err := db.Raw(func(raw any) error {
		info, err := raw.(interface {
			ColumnInfo(string) ([]modernsqlite.ColumnInfo, error)
		}).ColumnInfo(stmt)
		columnCount = len(info)
		return err
	})
	if err != nil {
		return ResultSet{
			Kind:      "error",
			Statement: stmt,
			Error:     err.Error(),
		}
	}
	if columnCount == 0 {
		result, err := db.ExecContext(ctx, stmt, args...)
		if err != nil {
			return ResultSet{
				Kind:      "error",
				Statement: stmt,
				Error:     err.Error(),
			}
		}
		affected, _ := result.RowsAffected()
		lastID, _ := result.LastInsertId()
		return ResultSet{
			Kind:         "status",
			Statement:    stmt,
			RowsAffected: affected,
			LastInsertID: safeCell(lastID),
		}
	}

	rows, err := db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return ResultSet{
			Kind:      "error",
			Statement: stmt,
			Error:     err.Error(),
		}
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return ResultSet{
			Kind:      "error",
			Statement: stmt,
			Error:     err.Error(),
		}
	}
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return ResultSet{
			Kind:      "error",
			Statement: stmt,
			Error:     err.Error(),
		}
	}
	columns := make([]ColumnMeta, len(colNames))
	for i, name := range colNames {
		declared := ""
		if colTypes[i] != nil {
			declared = colTypes[i].DatabaseTypeName()
		}
		columns[i] = ColumnMeta{Name: name, DeclaredType: declared}
	}

	// Give duplicate result labels distinct keys instead of losing earlier cells.
	used := make(map[string]bool)
	for i := range colNames {
		base := colNames[i]
		for suffix := 2; used[colNames[i]]; suffix++ {
			colNames[i] = fmt.Sprintf("%s (%d)", base, suffix)
		}
		used[colNames[i]] = true
		columns[i].Name = colNames[i]
	}
	var resultRows []map[string]any
	bytesUsed := 0
	dest := make([]any, len(colNames))
	ptrs := make([]any, len(colNames))
	for i := range dest {
		ptrs[i] = &dest[i]
	}
	truncated := false
	for rows.Next() {
		if len(resultRows) >= maxRows {
			truncated = true
			break
		}
		if err := rows.Scan(ptrs...); err != nil {
			return ResultSet{
				Kind:      "error",
				Statement: stmt,
				Error:     err.Error(),
			}
		}
		rawBytes := 0
		for _, v := range dest {
			switch v := v.(type) {
			case string:
				rawBytes += len(v)
			case []byte:
				rawBytes += base64.StdEncoding.EncodedLen(len(v))
			default:
				rawBytes += 16
			}
		}
		if rawBytes > maxBytes-bytesUsed {
			truncated = true
			break
		}
		row := make(map[string]any, len(colNames))
		for i, name := range colNames {
			row[name] = safeCell(dest[i])
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return ResultSet{
				Kind:      "error",
				Statement: stmt,
				Error:     err.Error(),
			}
		}
		if bytesUsed+len(encoded) > maxBytes {
			truncated = true
			break
		}
		bytesUsed += len(encoded)
		resultRows = append(resultRows, row)
	}
	if err := rows.Err(); err != nil {
		return ResultSet{
			Kind:      "error",
			Statement: stmt,
			Error:     err.Error(),
		}
	}

	return ResultSet{
		Kind:      "rows",
		Statement: stmt,
		Columns:   columns,
		Rows:      resultRows,
		Truncated: truncated,
	}
}

// safeCell converts a driver value into a JSON-safe representation:
// out-of-JS-safe-range integers become decimal strings, blobs become a
// {blob,size,base64} placeholder, everything else passes through.
func safeCell(v any) any {
	switch value := v.(type) {
	case int64:
		if value > maxSafeInt || value < -maxSafeInt {
			return strconv.FormatInt(value, 10)
		}
		return value
	case float64:
		if math.IsInf(value, 0) || math.IsNaN(value) {
			return strconv.FormatFloat(value, 'g', -1, 64)
		}
		return value
	case []byte:
		return BlobValue{Blob: true, Size: len(value), Base64: base64.StdEncoding.EncodeToString(value)}
	default:
		return value
	}
}
