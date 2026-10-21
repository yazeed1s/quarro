package dialect

import (
	"fmt"
	"strconv"
	"strings"
)

// paramToken is one ":name" placeholder occurrence, found outside quoted
// strings, comments, and PostgreSQL dollar-quoted bodies.
type paramToken struct {
	name       string
	start, end int
}

// scanParams walks sqlText once, skipping quoted strings and comments the
// same way Split does, and collects every ":name" placeholder. "::" (a
// PostgreSQL type cast) is never treated as a placeholder.
func scanParams(pg bool, s string) ([]paramToken, error) {
	var tokens []paramToken
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '\'' || c == '"' || (!pg && c == '`') {
			quote := c
			escape := !pg ||
				(i > 0 && (s[i-1] == 'e' || s[i-1] == 'E'))
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
				return nil, fmt.Errorf("unterminated quoted value")
			}
			continue
		}
		if strings.HasPrefix(s[i:], "--") &&
			(pg || i+2 == len(s) || s[i+2] <= ' ') {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if !pg && c == '#' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if strings.HasPrefix(s[i:], "/*") {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("unterminated block comment")
			}
			i += 2 + end + 2
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
					return nil, fmt.Errorf(
						"unterminated dollar quote",
					)
				}
				i = j + 1 + end + len(tag)
				continue
			}
		}
		if c == ':' {
			// "::" is a PostgreSQL type cast, never a parameter.
			if i+1 < len(s) && s[i+1] == ':' {
				i += 2
				continue
			}
			j := i + 1
			for j < len(s) && (s[j] == '_' || s[j] >= 'a' && s[j] <= 'z' || s[j] >= 'A' && s[j] <= 'Z' || s[j] >= '0' && s[j] <= '9') {
				j++
			}
			if j > i+1 {
				tokens = append(
					tokens,
					paramToken{name: s[i+1 : j], start: i, end: j},
				)
				i = j
				continue
			}
		}
		i++
	}
	return tokens, nil
}

// PlaceholderNames returns the distinct ":name" parameters in sqlText, in
// order of first appearance, so the frontend can prompt for values before
// running a parameterized statement. pg selects PostgreSQL's quoting/comment
// rules (dollar quoting, "--" always a comment) over every other engine's.
func PlaceholderNames(pg bool, s string) ([]string, error) {
	tokens, err := scanParams(pg, s)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	names := []string{}
	for _, t := range tokens {
		if !seen[t.name] {
			seen[t.name] = true
			names = append(names, t.name)
		}
	}
	return names, nil
}

// coerceParam turns a string parameter value into an integer or decimal when
// it parses cleanly as one, so numeric comparisons on the server behave as
// expected; otherwise it is bound as text. Values are always passed through
// database/sql args, never interpolated into the statement.
func coerceParam(v any) any {
	s, ok := v.(string)
	if !ok || s == "" {
		return v
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

// BindParams rewrites every ":name" placeholder into the dialect's own bound
// parameter syntax (via placeholder) and returns the ordered driver args.
// Every occurrence of a name becomes its own positional arg, so repeated
// placeholders work under drivers that only support positional binding.
func BindParams(
	pg bool,
	placeholder func(int) string,
	s string,
	values map[string]any,
) (string, []any, error) {
	tokens, err := scanParams(pg, s)
	if err != nil {
		return "", nil, err
	}
	if len(tokens) == 0 {
		return s, nil, nil
	}
	var b strings.Builder
	args := make([]any, 0, len(tokens))
	last := 0
	for _, t := range tokens {
		v, ok := values[t.name]
		if !ok {
			return "", nil, fmt.Errorf(
				"missing value for parameter :%s",
				t.name,
			)
		}
		b.WriteString(s[last:t.start])
		args = append(args, coerceParam(v))
		b.WriteString(placeholder(len(args)))
		last = t.end
	}
	b.WriteString(s[last:])
	return b.String(), args, nil
}
