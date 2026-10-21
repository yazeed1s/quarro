package dialect

import (
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/yazeed1s/quarro/internal/database/model"
)

// Split splits sqlText into individually executable statements, respecting
// PostgreSQL dollar quoting when pg is true and MySQL DELIMITER directives
// and backtick identifiers when it is false. Offsets in the returned ranges
// are UTF-16 positions in the editor, never Go byte offsets.
func Split(
	pg bool,
	s string,
) ([]model.StatementRange, []string, error) {
	var ranges []model.StatementRange
	var texts []string
	start := 0
	delimiter := ";"
	hasSQL := false
	add := func(end, rangeEnd int) {
		if hasSQL {
			ranges = append(
				ranges,
				model.StatementRange{
					Start: len(utf16.Encode([]rune(s[:start]))),
					End:   len(utf16.Encode([]rune(s[:rangeEnd]))),
					Text:  strings.TrimSpace(s[start:end]),
				},
			)
			texts = append(texts, strings.TrimSpace(s[start:end]))
		}
		hasSQL = false
	}
	for i := 0; i < len(s); {
		if !pg && !hasSQL && (i == 0 || s[i-1] == '\n') {
			end := strings.IndexByte(s[i:], '\n')
			if end < 0 {
				end = len(s) - i
			}
			line := strings.TrimSpace(s[i : i+end])
			fields := strings.Fields(line)
			if len(fields) > 0 &&
				strings.EqualFold(fields[0], "DELIMITER") {
				if len(fields) != 2 || len(fields[1]) > 16 {
					return nil, nil, fmt.Errorf(
						"invalid DELIMITER directive",
					)
				}
				delimiter = fields[1]
				i += end
				if i < len(s) {
					i++
				}
				start = i
				continue
			}
		}
		if strings.HasPrefix(s[i:], delimiter) {
			add(i, i+len(delimiter))
			i += len(delimiter)
			start = i
			continue
		}
		c := s[i]
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			i++
			continue
		}
		if strings.HasPrefix(s[i:], "--") &&
			(pg || i+2 == len(s) || s[i+2] <= ' ') ||
			!pg && c == '#' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if strings.HasPrefix(s[i:], "/*") {
			if !pg && strings.HasPrefix(s[i:], "/*!") {
				hasSQL = true
			}
			depth := 1
			i += 2
			for i < len(s) && depth > 0 {
				if strings.HasPrefix(s[i:], "*/") {
					depth--
					i += 2
				} else if pg && strings.HasPrefix(s[i:], "/*") {
					depth++
					i += 2
				} else {
					i++
				}
			}
			if depth > 0 {
				return nil, nil, fmt.Errorf(
					"unterminated block comment",
				)
			}
			continue
		}
		hasSQL = true
		if c == '\'' || c == '"' || !pg && c == '`' {
			quote := c
			escape := !pg || i > 0 && (s[i-1] == 'e' || s[i-1] == 'E')
			i++
			closed := false
			for i < len(s) {
				if escape && s[i] == '\\' {
					i += 2
					continue
				}
				if s[i] == quote {
					i++
					if i < len(s) && s[i] == quote {
						i++
						continue
					}
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, nil, fmt.Errorf(
					"unterminated quoted value",
				)
			}
			continue
		}
		if pg && c == '$' && (i == 0 || !identifierByte(s[i-1])) {
			j := i + 1
			for j < len(s) && (s[j] == '_' || s[j] >= 'a' && s[j] <= 'z' || s[j] >= 'A' && s[j] <= 'Z' || j > i+1 && s[j] >= '0' && s[j] <= '9') {
				j++
			}
			if j < len(s) && s[j] == '$' {
				tag := s[i : j+1]
				end := strings.Index(s[j+1:], tag)
				if end < 0 {
					return nil, nil, fmt.Errorf(
						"unterminated dollar quote",
					)
				}
				i = j + 1 + end + len(tag)
				continue
			}
		}
		i++
	}
	add(len(s), len(s))
	return ranges, texts, nil
}

func identifierByte(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' ||
		c >= 'A' && c <= 'Z' ||
		c >= '0' && c <= '9' ||
		c >= 128
}
