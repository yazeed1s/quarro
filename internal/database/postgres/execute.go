package postgres

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
)

func failure(stmt string, e error) model.ResultSet {
	result := dialect.Failure(stmt, e)
	var pg *pgconn.PgError
	if errors.As(e, &pg) {
		result.ErrorCode = pg.Code
	}
	return result
}

func (Provider) Execute(
	ctx context.Context,
	c any,
	stmt string,
	n, b int,
	args ...any,
) model.ResultSet {
	conn := c.(*sql.Conn)
	if n < 1 || n > 10000 {
		n = 500
	}
	result := model.ResultSet{
		Kind:      "rows",
		Statement: stmt,
		Rows:      []map[string]any{},
	}
	lost := false
	e := conn.Raw(func(raw any) error {
		defer func() { lost = raw.(*stdlib.Conn).Conn().IsClosed() }()
		rows, e := raw.(*stdlib.Conn).Conn().
			Query(ctx, stmt, append([]any{pgx.QueryResultFormats{pgx.TextFormatCode}}, args...)...)
		if e != nil {
			return e
		}
		defer rows.Close()
		fields := rows.FieldDescriptions()
		rawNames := []string{}
		for _, f := range fields {
			rawNames = append(rawNames, f.Name)
		}
		keys := dialect.Names(rawNames)
		for i, f := range fields {
			typ := strconv.Itoa(int(f.DataTypeOID))
			if t, ok := raw.(*stdlib.Conn).Conn().TypeMap().TypeForOID(f.DataTypeOID); ok {
				typ = t.Name
			}
			result.Columns = append(
				result.Columns,
				model.ColumnMeta{Name: keys[i], DeclaredType: typ},
			)
		}
		for rows.Next() {
			rawValues := rows.RawValues()
			size := 0
			for _, v := range rawValues {
				size += len(v)
			}
			if size > b {
				result.Truncated = true
				break
			}
			// Use wire text for exact numeric, interval, array and temporal values.
			values, e := rows.Values()
			if e != nil {
				return e
			}
			for i, v := range values {
				if rawValues[i] == nil {
					values[i] = nil
					continue
				}
				switch fields[i].DataTypeOID {
				case 16, 20, 21, 23, 700, 701:
					values[i] = v
				case 17:
					x, ok := v.([]byte)
					if !ok {
						return errInvalidBytea
					}
					values[i] = model.BlobValue{
						Blob:   true,
						Size:   len(x),
						Base64: base64.StdEncoding.EncodeToString(x),
					}
				default:
					values[i] = string(rawValues[i])
				}
			}
			if !dialect.AppendRow(&result, keys, values, n, &b) {
				break
			}
		}
		rows.Close()
		if e := rows.Err(); e != nil {
			return e
		}
		if len(fields) == 0 {
			result.Kind = "status"
			result.Rows = nil
			result.RowsAffected = rows.CommandTag().RowsAffected()
		}
		return nil
	})
	if e != nil {
		result := failure(stmt, e)
		result.ConnectionLost = lost
		if lost {
			result.Error += " The connection was closed; reconnect before continuing. Transaction outcome may be unknown."
		}
		return result
	}
	return result
}

func (p Provider) ExecuteResults(
	ctx context.Context,
	conn any,
	stmt string,
	n, b int,
	args ...any,
) []model.ResultSet {
	return []model.ResultSet{
		p.Execute(ctx, conn, stmt, n, b, args...),
	}
}

var errInvalidBytea = errors.New("invalid bytea value")
