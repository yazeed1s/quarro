package dialect

import (
	"encoding/json"
	"fmt"
)

// ObjectID encodes a schema-qualified object (a PostgreSQL/MySQL table or
// view) as an opaque JSON pair, avoiding ambiguity for dots inside schema or
// table names.
func ObjectID(schema, name string) string {
	b, _ := json.Marshal([]string{schema, name})
	return string(b)
}

// ParseObject decodes an id produced by ObjectID.
func ParseObject(id string) (schema, name string, err error) {
	var v []string
	if json.Unmarshal([]byte(id), &v) != nil || len(v) != 2 ||
		v[0] == "" ||
		v[1] == "" {
		return "", "", fmt.Errorf("invalid qualified object")
	}
	return v[0], v[1], nil
}
