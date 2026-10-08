package test

import "testing"

// The harness drops the whole public schema; it must refuse anything that
// isn't explicitly a test database.
func TestSchemaResetOnlyTouchesTestDatabases(t *testing.T) {
	for name, want := range map[string]bool{
		"zawaj_test":  true,
		"ci_test":     true,
		"zawaj":       false,
		"prod":        false,
		"test_zawaj":  false,
		"zawaj_test2": false,
	} {
		if got := isTestDatabase(name); got != want {
			t.Errorf("isTestDatabase(%q) = %v, want %v", name, got, want)
		}
	}
}
