package app

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// rowWriter streams one export format to an already-open file. Rows arrive
// exactly as browsing/execution already produced them (see model.ResultSet),
// so an out-of-JS-safe-range integer is a decimal string and a blob is a
// model.BlobValue, both render as plain, lossless text in every format
// export supports.
type rowWriter interface {
	WriteHeader(columns []model.ColumnMeta) error
	WriteRow(columns []model.ColumnMeta, row map[string]any) error
	Close() error
}

// blobBytes recognizes a blob cell in either shape it can arrive in: a
// model.BlobValue (from a JSON round trip already decoded into a Go struct)
// or the equivalent map[string]any (decoded from raw JSON).
func blobBytes(v any) ([]byte, bool) {
	switch x := v.(type) {
	case model.BlobValue:
		raw, err := base64.StdEncoding.DecodeString(x.Base64)
		return raw, err == nil
	case map[string]any:
		if b, _ := x["blob"].(bool); b {
			s, _ := x["base64"].(string)
			raw, err := base64.StdEncoding.DecodeString(s)
			return raw, err == nil
		}
	}
	return nil, false
}

func newRowWriter(
	format, delimiter string,
	quoteIdent func(string) string,
	literal func(any) (string, error),
	blobLiteral func([]byte) string,
	table string,
	w io.Writer,
) (rowWriter, error) {
	switch format {
	case "csv":
		return newCSVWriter(delimiter, w), nil
	case "json":
		return newJSONWriter(w), nil
	case "sql":
		return newSQLWriter(
			quoteIdent,
			literal,
			blobLiteral,
			table,
			w,
		), nil
	default:
		return nil, fmt.Errorf("unsupported export format %q", format)
	}
}

// cellText renders one cell as export-safe CSV text.
func cellText(v any) string {
	if raw, ok := blobBytes(v); ok {
		return base64.StdEncoding.EncodeToString(raw)
	}
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

type csvWriter struct {
	w *csv.Writer
}

func newCSVWriter(delimiter string, w io.Writer) *csvWriter {
	cw := csv.NewWriter(w)
	if r := []rune(delimiter); len(r) == 1 {
		cw.Comma = r[0]
	}
	return &csvWriter{w: cw}
}
func (c *csvWriter) WriteHeader(columns []model.ColumnMeta) error {
	names := make([]string, len(columns))
	for i, col := range columns {
		names[i] = col.Name
	}
	return c.w.Write(names)
}

func (c *csvWriter) WriteRow(
	columns []model.ColumnMeta,
	row map[string]any,
) error {
	fields := make([]string, len(columns))
	for i, col := range columns {
		fields[i] = cellText(row[col.Name])
	}
	return c.w.Write(fields)
}
func (c *csvWriter) Close() error {
	c.w.Flush()
	return c.w.Error()
}

// jsonWriter streams a JSON array by hand (rather than json.Marshal on a
// slice) so rows are written as they arrive instead of being buffered in
// memory for one giant encode.
type jsonWriter struct {
	w       io.Writer
	started bool
}

func newJSONWriter(
	w io.Writer,
) *jsonWriter {
	return &jsonWriter{w: w}
}
func (j *jsonWriter) WriteHeader([]model.ColumnMeta) error {
	_, err := j.w.Write([]byte("[\n"))
	return err
}

func (j *jsonWriter) WriteRow(
	columns []model.ColumnMeta,
	row map[string]any,
) error {
	if j.started {
		if _, err := j.w.Write([]byte(",\n")); err != nil {
			return err
		}
	}
	j.started = true
	ordered := make(map[string]any, len(columns))
	for _, col := range columns {
		ordered[col.Name] = jsonCell(row[col.Name])
	}
	data, err := json.Marshal(ordered)
	if err != nil {
		return err
	}
	_, err = j.w.Write(data)
	return err
}
func (j *jsonWriter) Close() error {
	_, err := j.w.Write([]byte("\n]\n"))
	return err
}

// jsonCell keeps numbers, strings and null as real JSON values, a
// decimal-string bigint from the row shape still renders as a JSON string,
// which is exact and precision-safe, and reduces a blob placeholder to its
// base64 text so JSON export never nests the {blob,size,base64} wrapper.
func jsonCell(v any) any {
	if raw, ok := blobBytes(v); ok {
		return base64.StdEncoding.EncodeToString(raw)
	}
	return v
}

// sqlWriter emits one INSERT statement per row using the target engine's own
// identifier quoting and literal encoding (see provider.QuoteIdentifier /
// provider.FormatLiteral) plus a small blob-literal formatter the caller
// supplies per engine, so a dump can be replayed on the same engine.
type sqlWriter struct {
	w           io.Writer
	quoteIdent  func(string) string
	literal     func(any) (string, error)
	blobLiteral func([]byte) string
	table       string
}

func newSQLWriter(
	quoteIdent func(string) string,
	literal func(any) (string, error),
	blobLiteral func([]byte) string,
	table string,
	w io.Writer,
) *sqlWriter {
	return &sqlWriter{
		w:           w,
		quoteIdent:  quoteIdent,
		literal:     literal,
		blobLiteral: blobLiteral,
		table:       table,
	}
}

func (s *sqlWriter) WriteHeader(
	[]model.ColumnMeta,
) error {
	return nil
}

func (s *sqlWriter) WriteRow(
	columns []model.ColumnMeta,
	row map[string]any,
) error {
	names := make([]string, len(columns))
	values := make([]string, len(columns))
	for i, col := range columns {
		names[i] = s.quoteIdent(col.Name)
		cell := row[col.Name]
		if raw, ok := blobBytes(cell); ok {
			values[i] = s.blobLiteral(raw)
			continue
		}
		literal, err := s.literal(cell)
		if err != nil {
			return fmt.Errorf("column %s: %w", col.Name, err)
		}
		values[i] = literal
	}
	stmt := "INSERT INTO " + s.table + " (" + strings.Join(
		names,
		", ",
	) + ") VALUES (" + strings.Join(
		values,
		", ",
	) + ");\n"
	_, err := s.w.Write([]byte(stmt))
	return err
}
func (s *sqlWriter) Close() error { return nil }
