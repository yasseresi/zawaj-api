package validation

import "testing"

// Keep these cases in sync with zawaj_app/test/unit/phone_test.dart.
func TestNormalizePhone(t *testing.T) {
	valid := map[string]string{
		"0672859965":        "0672859965",
		"0551234567":        "0551234567",
		"0771234567":        "0771234567",
		"021234567":         "021234567",
		"031234567":         "031234567",
		"041234567":         "041234567",
		"0672 85 99 65":     "0672859965",
		"0672-85-99-65":     "0672859965",
		"(0672) 85.99.65":   "0672859965",
		"+213672859965":     "0672859965",
		"+213 672 85 99 65": "0672859965",
		"00213672859965":    "0672859965",
		"213672859965":      "0672859965",
		"+21321234567":      "021234567",
	}
	for in, want := range valid {
		got, err := NormalizePhone(in)
		if err != nil || got != want {
			t.Errorf("NormalizePhone(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	invalid := []string{
		"", "   ", "123", "0551", "0812345678", "0112345678", "06728599651",
		"067285996", "02123456", "0212345678", "+33612345678", "abc0672859965",
	}
	for _, in := range invalid {
		if got, err := NormalizePhone(in); err == nil {
			t.Errorf("NormalizePhone(%q) = %q; want error", in, got)
		}
	}
}
