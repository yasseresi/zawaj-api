package validation

import (
	"fmt"
	"regexp"
	"strings"
)

// Algerian numbers in national format: mobile 0[5-7]+8 digits, fixed 0[2-4]+7.
// Keep in sync with zawaj_app/lib/src/utils/phone.dart.
var (
	phonePattern    = regexp.MustCompile(`^(0[5-7][0-9]{8}|0[2-4][0-9]{7})$`)
	phoneSeparators = strings.NewReplacer(" ", "", "-", "", ".", "", "(", "", ")", "")
)

// NormalizePhone returns an Algerian phone number in national format
// (e.g. "0672859965"). Separators are stripped and a +213 / 00213 / 213 country
// prefix is rewritten to the leading 0.
func NormalizePhone(value string) (string, error) {
	v := phoneSeparators.Replace(strings.TrimSpace(value))
	for _, prefix := range []string{"+213", "00213", "213"} {
		if strings.HasPrefix(v, prefix) {
			v = "0" + strings.TrimPrefix(v, prefix)
			break
		}
	}
	if !phonePattern.MatchString(v) {
		return "", fmt.Errorf("invalid phone number")
	}
	return v, nil
}
