package dialect

import "strings"

// QuoteDouble is the ANSI/SQLite/PostgreSQL identifier-quoting rule:
// wrap in double quotes, doubling any embedded quote.
func QuoteDouble(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// QuoteBacktick is MySQL's identifier-quoting rule: wrap in backticks,
// doubling any embedded backtick.
func QuoteBacktick(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}
