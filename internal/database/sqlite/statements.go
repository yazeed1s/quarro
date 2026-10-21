package sqlite

import (
	"unicode/utf16"

	"github.com/yazeed1s/quarro/internal/database/model"
	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

// StatementRange uses UTF-16 offsets, matching JavaScript and CodeMirror.
type StatementRange = model.StatementRange

// SplitStatements splits a SQL script into top-level statements, respecting
// quoted string/identifier literals and comments so a semicolon inside a
// string or comment never causes a false split. This is a tokenizing scan,
// not a full SQL parser: it classifies just enough syntax (quoting and
// comment forms) to find statement boundaries correctly.
func splitRuneRanges(sql string) []StatementRange {
	runes := []rune(sql)
	n := len(runes)
	var ranges []StatementRange
	start := -1
	i := 0
	for i < n {
		c := runes[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			if start < 0 {
				start = i
			}
			i = skipQuoted(runes, i, c)
		case c == '[':
			if start < 0 {
				start = i
			}
			i++
			for i < n && runes[i] != ']' {
				i++
			}
			if i < n {
				i++
			}
		case c == '-' && i+1 < n && runes[i+1] == '-':
			i += 2
			for i < n && runes[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && runes[i+1] == '*':
			i += 2
			for i+1 < n && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			i += 2
			if i > n {
				i = n
			}
		case c == ';':
			if start >= 0 && !complete(string(runes[start:i+1])) {
				i++
				continue
			}
			if start >= 0 {
				ranges = append(
					ranges,
					StatementRange{Start: start, End: i},
				)
			}
			start = -1
			i++
		default:
			if start < 0 && !isSQLSpace(c) {
				start = i
			}
			i++
		}
	}
	if start >= 0 {
		end := n
		for end > start && isSQLSpace(runes[end-1]) {
			end--
		}
		if end > start {
			ranges = append(
				ranges,
				StatementRange{Start: start, End: end},
			)
		}
	}
	return ranges
}

// skipQuoted advances past a quoted run starting at i (runes[i] == quote),
// honoring the SQL convention of a doubled quote as an escaped literal
// quote character, and returns the index just past the closing quote (or n
// if the quote is left unterminated).
func skipQuoted(runes []rune, i int, quote rune) int {
	n := len(runes)
	i++
	for i < n {
		if runes[i] == quote {
			if i+1 < n && runes[i+1] == quote {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return n
}

func isSQLSpace(c rune) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}

// SQLite's own completeness check handles trigger bodies and CASE/END.
func complete(sql string) bool {
	tls := libc.NewTLS()
	defer tls.Close()
	ptr, err := libc.CString(sql)
	if err != nil {
		return false
	}
	defer libc.Xfree(tls, ptr)
	return sqlite3.Xsqlite3_complete(tls, ptr) != 0
}

func SplitStatements(sql string) []StatementRange {
	runes := []rune(sql)
	offsets := make([]int, len(runes)+1)
	for i, r := range runes {
		offsets[i+1] = offsets[i] + len(utf16.Encode([]rune{r}))
	}
	ranges := splitRuneRanges(sql)
	for i := range ranges {
		ranges[i] = StatementRange{
			Start: offsets[ranges[i].Start],
			End:   offsets[ranges[i].End],
		}
	}
	return ranges
}

func StatementTexts(sql string) []string {
	runes := []rune(sql)
	var out []string
	for _, r := range splitRuneRanges(sql) {
		out = append(out, string(runes[r.Start:r.End]))
	}
	return out
}
