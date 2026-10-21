package app

import "testing"

func TestRejectReadOnlyOverride(t *testing.T) {
	refused := []string{
		"PRAGMA query_only = OFF",
		`PRAGMA "query_only" = OFF`,
		"SET default_transaction_read_only = off",
		"SET SESSION transaction_read_only = 0",
		"SET tx_read_only = false",
		"BEGIN READ WRITE",
		"START TRANSACTION READ WRITE",
		"RESET ALL",
		"DISCARD ALL",
		"SELECT set_config('default_transaction_read_only', 'off', false)",
	}
	for _, sql := range refused {
		if err := rejectReadOnlyOverride(sql); err == nil {
			t.Errorf("rejectReadOnlyOverride(%q) allowed an override", sql)
		}
	}
	allowed := []string{
		"SELECT * FROM items",
		"WITH rows AS (SELECT 1) SELECT * FROM rows",
		"SET statement_timeout = 1000",
		"SELECT 'SET default_transaction_read_only = off'",
		"-- PRAGMA query_only = OFF\nSELECT 1",
	}
	for _, sql := range allowed {
		if err := rejectReadOnlyOverride(sql); err != nil {
			t.Errorf("rejectReadOnlyOverride(%q) = %v", sql, err)
		}
	}
}
