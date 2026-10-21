package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	unicodeencoding "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"github.com/yazeed1s/quarro/internal/database/model"
)

var decimalPattern = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$`)
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$`)

func decodedImportReader(r io.Reader, enc string) (io.Reader, error) {
	br := bufio.NewReader(r)
	bom, _ := br.Peek(3)
	src := enc
	enc = strings.ToLower(strings.TrimSpace(enc))
	if enc == "" || enc == "auto" {
		enc = "utf-8"
		if len(bom) >= 2 && bom[0] == 0xff && bom[1] == 0xfe {
			enc = "utf-16le"
		} else if len(bom) >= 2 && bom[0] == 0xfe && bom[1] == 0xff {
			enc = "utf-16be"
		}
	}
	switch enc {
	case "utf-8":
		if len(bom) >= 3 && bom[0] == 0xef && bom[1] == 0xbb && bom[2] == 0xbf {
			_, _ = br.Discard(3)
		}
		return br, nil
	case "utf-16le":
		if len(bom) >= 2 && bom[0] == 0xff && bom[1] == 0xfe {
			_, _ = br.Discard(2)
		}
		return transform.NewReader(br, unicodeencoding.UTF16(unicodeencoding.LittleEndian, unicodeencoding.IgnoreBOM).NewDecoder()), nil
	case "utf-16be":
		if len(bom) >= 2 && bom[0] == 0xfe && bom[1] == 0xff {
			_, _ = br.Discard(2)
		}
		return transform.NewReader(br, unicodeencoding.UTF16(unicodeencoding.BigEndian, unicodeencoding.IgnoreBOM).NewDecoder()), nil
	case "windows-1252":
		return transform.NewReader(br, charmap.Windows1252.NewDecoder()), nil
	case "iso-8859-1":
		return transform.NewReader(br, charmap.ISO8859_1.NewDecoder()), nil
	default:
		return nil, fmt.Errorf("unsupported CSV encoding %q", src)
	}
}

func validateUTF8Record(rec []string) error {
	for i, v := range rec {
		if !utf8.ValidString(v) {
			return fmt.Errorf("column %d contains invalid UTF-8; choose the file's source encoding", i+1)
		}
	}
	return nil
}

func importValue(col model.Column, raw string) (any, bool, error) {
	typ := strings.ToLower(strings.TrimSpace(col.Type))
	ws := strings.FieldsFunc(typ, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') })
	has := func(set ...string) bool {
		for _, w := range ws {
			for _, v := range set {
				if w == v {
					return true
				}
			}
		}
		return false
	}
	if has("tinyint", "smallint", "mediumint", "int", "integer", "bigint", "serial", "bigserial", "smallserial") {
		v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return nil, true, fmt.Errorf("%q is not a valid integer", raw)
		}
		return v, true, nil
	}
	if has("numeric", "decimal", "real", "float", "double", "money") {
		if !decimalPattern.MatchString(strings.TrimSpace(raw)) {
			return nil, true, fmt.Errorf("%q is not a valid number", raw)
		}
		return raw, true, nil
	}
	if has("bool", "boolean") {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "true", "t", "1", "yes", "y":
			return true, true, nil
		case "false", "f", "0", "no", "n":
			return false, true, nil
		default:
			return nil, true, fmt.Errorf("%q is not a valid boolean", raw)
		}
	}
	if has("json", "jsonb") {
		if !json.Valid([]byte(raw)) {
			return nil, true, fmt.Errorf("value is not valid JSON")
		}
		return raw, true, nil
	}
	if has("uuid") {
		if !uuidPattern.MatchString(strings.TrimSpace(raw)) {
			return nil, true, fmt.Errorf("%q is not a valid UUID", raw)
		}
		return raw, true, nil
	}
	if has("date") && !has("datetime") {
		if _, err := time.Parse("2006-01-02", strings.TrimSpace(raw)); err != nil {
			return nil, true, fmt.Errorf("%q is not a valid date (expected YYYY-MM-DD)", raw)
		}
		return raw, true, nil
	}
	if has("timestamp", "datetime") {
		v := strings.TrimSpace(raw)
		fmts := []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"}
		for _, f := range fmts {
			if _, err := time.Parse(f, v); err == nil {
				return raw, true, nil
			}
		}
		return nil, true, fmt.Errorf("%q is not a valid timestamp", raw)
	}
	if has("time") && !has("timestamp", "datetime") {
		v := strings.TrimSpace(raw)
		for _, f := range []string{"15:04:05.999999999Z07:00", "15:04:05Z07:00", "15:04:05.999999999", "15:04:05"} {
			if _, err := time.Parse(f, v); err == nil {
				return raw, true, nil
			}
		}
		return nil, true, fmt.Errorf("%q is not a valid time", raw)
	}
	if has("char", "character", "varchar", "text", "clob", "blob", "bytea", "binary", "varbinary") {
		return raw, true, nil
	}
	return raw, false, nil
}
