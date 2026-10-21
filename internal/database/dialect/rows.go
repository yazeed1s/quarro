package dialect

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// Failure builds an error result for stmt, classifying the generic
// database/sql/context outcomes every engine shares (cancellation, timeout,
// a connection the driver has already given up on). Engine-specific error
// codes (a PostgreSQL SQLSTATE, a MySQL error number) are layered on by the
// caller after this returns, since only the engine package knows its own
// driver's error types.
func Failure(stmt string, e error) model.ResultSet {
	result := model.ResultSet{
		Kind:      "error",
		Statement: stmt,
		Error:     e.Error(),
		ErrorCode: "database_error",
	}
	switch {
	case errors.Is(e, context.Canceled):
		result.ErrorCode = "cancelled"
	case errors.Is(e, context.DeadlineExceeded):
		result.ErrorCode = "timeout"
	}
	result.ConnectionLost = errors.Is(e, driver.ErrBadConn) ||
		errors.Is(e, sql.ErrConnDone)
	return result
}

// Safe converts a driver value into a JSON-safe representation: integers
// outside JavaScript's safe range become decimal strings, non-finite floats
// become their Go string form, and timestamps become RFC3339Nano text.
func Safe(v any) any {
	switch x := v.(type) {
	case int64:
		if x > 1<<53-1 || x < -(1<<53-1) {
			return strconv.FormatInt(x, 10)
		}
	case uint64:
		if x > 1<<53-1 {
			return strconv.FormatUint(x, 10)
		}
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return fmt.Sprint(x)
		}
	case time.Time:
		return x.Format(time.RFC3339Nano)
	}
	return v
}

// Names disambiguates duplicate result-column labels (e.g. "SELECT 1 AS x,2
// AS x") by appending " (n)" to later occurrences, so no column silently
// overwrites another in the row map.
func Names(raw []string) []string {
	out := make([]string, len(raw))
	seen := map[string]bool{}
	for i, n := range raw {
		key := n
		for j := 2; seen[key]; j++ {
			key = fmt.Sprintf("%s (%d)", n, j)
		}
		seen[key] = true
		out[i] = key
	}
	return out
}

// AppendRow adds one row (keys paired with values) to result if it fits
// within the row-count limit n and the remaining byte budget. It reports
// whether the row was appended; false means the caller should stop scanning
// (result.Truncated is already set, or result.Kind was set to "error").
func AppendRow(
	result *model.ResultSet,
	keys []string,
	values []any,
	n int,
	budget *int,
) bool {
	if len(result.Rows) >= n {
		result.Truncated = true
		return false
	}
	row := map[string]any{}
	for i, v := range values {
		row[keys[i]] = Safe(v)
	}
	data, e := json.Marshal(row)
	if e != nil {
		result.Kind = "error"
		result.Error = e.Error()
		return false
	}
	if len(data) > *budget {
		result.Truncated = true
		return false
	}
	*budget -= len(data)
	result.Rows = append(result.Rows, row)
	return true
}

// FirstKeyword returns the first SQL keyword in s, skipping leading
// whitespace and comments (including MySQL's "#" line comments and "/*!
// ... */" executable comments, which are never stripped since MySQL runs
// their contents).
func FirstKeyword(s string) string {
	for {
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, "/*") &&
			!strings.HasPrefix(s, "/*!") {
			end := strings.Index(s[2:], "*/")
			if end < 0 {
				return ""
			}
			s = s[end+4:]
			continue
		}
		if strings.HasPrefix(s, "#") || strings.HasPrefix(s, "-- ") {
			end := strings.IndexByte(s, '\n')
			if end < 0 {
				return ""
			}
			s = s[end+1:]
			continue
		}
		break
	}
	end := 0
	for end < len(s) && (s[end] >= 'a' && s[end] <= 'z' || s[end] >= 'A' && s[end] <= 'Z') {
		end++
	}
	return strings.ToUpper(s[:end])
}
