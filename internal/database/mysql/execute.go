package mysql

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
)

func failure(stmt string, e error) model.ResultSet {
	result := dialect.Failure(stmt, e)
	var my *mysqldriver.MySQLError
	if errors.As(e, &my) {
		result.ErrorCode = fmt.Sprintf(
			"%s/%d",
			my.SQLState,
			my.Number,
		)
	}
	return result
}

// Execute delegates to ExecuteResults and returns its first result set, per
// the SQLExecutor contract's single-result-set method; MySQL statements that
// can return more than one (a stored-procedure CALL) are reached through
// ExecuteResults instead.
func (p Provider) Execute(
	ctx context.Context,
	conn any,
	stmt string,
	n, b int,
	args ...any,
) model.ResultSet {
	return p.ExecuteResults(ctx, conn, stmt, n, b, args...)[0]
}

func (Provider) ExecuteResults(
	ctx context.Context,
	c any,
	stmt string,
	n, b int,
	args ...any,
) []model.ResultSet {
	conn := c.(*sql.Conn)
	if n < 1 || n > 10000 {
		n = 500
	}
	failed := func(e error) model.ResultSet {
		result := failure(stmt, e)
		if ctx.Err() != nil {
			result.ConnectionLost = true
			result.Error += " The connection was closed; reconnect before continuing. Transaction outcome may be unknown."
		}
		return result
	}
	// MySQL INSERT/REPLACE never return rows. Use the driver's per-command
	// insert ID rather than LAST_INSERT_ID(), which can retain an older ID.
	keyword := dialect.FirstKeyword(stmt)
	if keyword == "INSERT" || keyword == "REPLACE" {
		result, e := conn.ExecContext(ctx, stmt, args...)
		if e != nil {
			return []model.ResultSet{failed(e)}
		}
		affected, _ := result.RowsAffected()
		id, _ := result.LastInsertId()
		return []model.ResultSet{
			{
				Kind:         "status",
				Statement:    stmt,
				RowsAffected: affected,
				LastInsertID: dialect.Safe(id),
			},
		}
	}
	rows, e := conn.QueryContext(ctx, stmt, args...)
	if e != nil {
		return []model.ResultSet{failed(e)}
	}
	defer rows.Close()
	results := []model.ResultSet{}
	for {
		if len(results) >= 100 {
			results = append(
				results,
				model.ResultSet{
					Kind:  "error",
					Error: "Result set limit reached (100).",
				},
			)
			break
		}
		result := model.ResultSet{
			Kind:      "rows",
			Statement: stmt,
			Rows:      []map[string]any{},
		}
		cols, e := rows.Columns()
		if e != nil {
			return append(results, failed(e))
		}
		types, e := rows.ColumnTypes()
		if e != nil {
			return append(results, failed(e))
		}
		keys := dialect.Names(cols)
		for i, t := range types {
			result.Columns = append(
				result.Columns,
				model.ColumnMeta{
					Name:         keys[i],
					DeclaredType: t.DatabaseTypeName(),
				},
			)
		}
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		for rows.Next() {
			if e := rows.Scan(ptrs...); e != nil {
				return append(results, failed(e))
			}
			size := 0
			for _, v := range values {
				switch v := v.(type) {
				case []byte:
					size += len(v)
				case string:
					size += len(v)
				}
			}
			if size > b {
				result.Truncated = true
				break
			}
			for i, v := range values {
				if x, ok := v.([]byte); ok {
					typ := strings.ToUpper(
						types[i].DatabaseTypeName(),
					)
					if strings.Contains(typ, "BLOB") ||
						strings.Contains(typ, "BINARY") ||
						typ == "BIT" {
						values[i] = model.BlobValue{
							Blob: true,
							Size: len(x),
							Base64: base64.StdEncoding.EncodeToString(
								x,
							),
						}
					} else {
						values[i] = string(x)
					}
				}
			}
			if !dialect.AppendRow(&result, keys, values, n, &b) {
				break
			}
		}
		if e := rows.Err(); e != nil {
			return append(results, failed(e))
		}
		if len(cols) == 0 {
			result.Kind = "status"
			result.Rows = nil
		}
		results = append(results, result)
		if !rows.NextResultSet() {
			break
		}
	}
	if e := rows.Err(); e != nil {
		return append(results, failed(e))
	}
	if e := rows.Close(); e != nil {
		return append(results, failed(e))
	}
	if len(results) == 1 && results[0].Kind == "status" {
		var affected int64
		if e := conn.QueryRowContext(ctx, "SELECT ROW_COUNT()").Scan(&affected); e != nil {
			return []model.ResultSet{failed(e)}
		}
		if affected > 0 {
			results[0].RowsAffected = affected
		}
	}
	return results
}
