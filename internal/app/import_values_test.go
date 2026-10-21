package app

import (
	"testing"

	"github.com/yazeed1s/quarro/internal/database/model"
)

func TestImportValueValidation(t *testing.T) {
	tcs := []struct {
		name    string
		typ     string
		value   string
		checked bool
		valid   bool
	}{
		{name: "integer", typ: "INTEGER", value: "42", checked: true, valid: true},
		{name: "bad integer", typ: "bigint", value: "4.2", checked: true, valid: false},
		{name: "decimal exponent", typ: "numeric", value: "1.25e3", checked: true, valid: true},
		{name: "boolean", typ: "BOOLEAN", value: "yes", checked: true, valid: true},
		{name: "bad boolean", typ: "bool", value: "sometimes", checked: true, valid: false},
		{name: "date", typ: "DATE", value: "2026-10-20", checked: true, valid: true},
		{name: "bad date", typ: "DATE", value: "2026-02-30", checked: true, valid: false},
		{name: "timestamp", typ: "timestamp with time zone", value: "2026-10-20T13:45:00-04:00", checked: true, valid: true},
		{name: "time zone", typ: "time with time zone", value: "13:45:00-04:00", checked: true, valid: true},
		{name: "json", typ: "JSONB", value: `{"ok":true}`, checked: true, valid: true},
		{name: "bad json", typ: "JSON", value: `{no}`, checked: true, valid: false},
		{name: "uuid", typ: "UUID", value: "123e4567-e89b-12d3-a456-426614174000", checked: true, valid: true},
		{name: "text", typ: "character varying", value: "anything", checked: true, valid: true},
		{name: "custom type", typ: "customer_status", value: "active", checked: false, valid: true},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			_, ok, err := importValue(model.Column{Type: tc.typ}, tc.value)
			if ok != tc.checked || (err == nil) != tc.valid {
				t.Fatalf("checked=%v err=%v", ok, err)
			}
		})
	}
}
