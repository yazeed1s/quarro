package app

import (
	"fmt"
	"strings"
	"unicode"
)

// rejectReadOnlyOverride blocks the small set of control statements that can
// turn off the database-side guard Quarro installs on a read-only connection.
// Data-changing statements themselves are left to the database's native
// read-only mode, which also catches writes hidden inside functions/triggers.
func rejectReadOnlyOverride(stmt string) error {
	words := sqlControlWords(stmt)
	if len(words) == 0 {
		return nil
	}
	contains := func(want string) bool {
		for _, word := range words {
			if word == want {
				return true
			}
		}
		return false
	}
	containsPair := func(a, b string) bool {
		for i := 0; i+1 < len(words); i++ {
			if words[i] == a && words[i+1] == b {
				return true
			}
		}
		return false
	}
	setting := contains("TRANSACTION_READ_ONLY") ||
		contains("DEFAULT_TRANSACTION_READ_ONLY") ||
		contains("TX_READ_ONLY") || contains("QUERY_ONLY")
	unsafe := setting && (words[0] == "SET" || words[0] == "RESET" ||
		words[0] == "PRAGMA")
	// PostgreSQL's set_config accepts expressions, so inspecting its quoted
	// arguments here would still be bypassable. Refuse the function itself in
	// read-only sessions; ordinary SETs for unrelated settings remain valid.
	unsafe = unsafe || contains("SET_CONFIG")
	unsafe = unsafe || ((words[0] == "BEGIN" || words[0] == "START" ||
		words[0] == "SET") && containsPair("READ", "WRITE"))
	unsafe = unsafe || (words[0] == "RESET" && contains("ALL")) ||
		(words[0] == "DISCARD" && contains("ALL"))
	if unsafe {
		return fmt.Errorf("this connection is read-only; changing its read-only mode is not allowed")
	}
	return nil
}

// sqlControlWords returns upper-cased SQL words while omitting comments and
// string literals. Quoted identifiers are retained so a quoted query_only (or
// equivalent) cannot evade the guard. This is deliberately a control-statement
// scanner, not a write classifier; the database remains the write authority.
func sqlControlWords(sql string) []string {
	words := []string{}
	for i := 0; i < len(sql); {
		if unicode.IsSpace(rune(sql[i])) || strings.ContainsRune("(),;=", rune(sql[i])) {
			i++
			continue
		}
		if i+1 < len(sql) && sql[i:i+2] == "--" {
			if end := strings.IndexByte(sql[i+2:], '\n'); end >= 0 {
				i += end + 3
			} else {
				break
			}
			continue
		}
		if i+1 < len(sql) && sql[i:i+2] == "/*" {
			if end := strings.Index(sql[i+2:], "*/"); end >= 0 {
				i += end + 4
			} else {
				break
			}
			continue
		}
		if sql[i] == '\'' {
			quote := sql[i]
			i++
			for i < len(sql) {
				if sql[i] == quote {
					if i+1 < len(sql) && sql[i+1] == quote {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			continue
		}
		if sql[i] == '"' || sql[i] == '`' || sql[i] == '[' {
			quote := sql[i]
			closeQuote := quote
			if quote == '[' {
				closeQuote = ']'
			}
			i++
			start := i
			for i < len(sql) && sql[i] != closeQuote {
				i++
			}
			if start < i {
				words = append(words, strings.ToUpper(sql[start:i]))
			}
			if i < len(sql) {
				i++
			}
			continue
		}
		start := i
		for i < len(sql) && (unicode.IsLetter(rune(sql[i])) || unicode.IsDigit(rune(sql[i])) || sql[i] == '_' || sql[i] == '@') {
			i++
		}
		if start == i {
			i++
			continue
		}
		words = append(words, strings.ToUpper(sql[start:i]))
	}
	return words
}
