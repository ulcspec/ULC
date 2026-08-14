package sheet

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestConversionPolicyDocMatchesConverter keeps the published conversion policy
// and the converter that implements it from drifting apart. The constants are
// bound to the CODE, not restated from the document, so editing a factor in
// units.go reddens this guard rather than silently making the published policy
// wrong. companionValue and siValue are the two functions the document
// describes.
func TestConversionPolicyDocMatchesConverter(t *testing.T) {
	path := filepath.Join(filepath.Dir(schemaDir(t)), "docs", "conversion-policy.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read conversion-policy.md: %v", err)
	}
	doc := string(raw)

	for _, c := range []struct {
		name  string
		value float64
	}{
		{"mmPerInch", mmPerInch},
		{"lbPerKg", lbPerKg},
		{"ft2PerM2", ft2PerM2},
		{"lbPerFtPerKgPerM", lbPerFtPerKgPerM},
	} {
		want := strconv.FormatFloat(c.value, 'f', -1, 64)
		if !strings.Contains(doc, want) {
			t.Errorf("docs/conversion-policy.md does not carry the %s constant %s", c.name, want)
		}
	}

	for _, phrase := range []string{
		"round half away from zero",
		"at most 4 decimal places",
		"1 decimal place",
		"verbatim",
	} {
		if !strings.Contains(doc, phrase) {
			t.Errorf("docs/conversion-policy.md does not state %q", phrase)
		}
	}

	// The worked example is bound by behavior, not by restating a literal.
	if got := dualLength.companionValue(113); got != 4.4488 {
		t.Fatalf("companionValue(113 mm) = %v, want 4.4488 (the document's worked example)", got)
	}
	if !strings.Contains(doc, "4.4488") {
		t.Error("docs/conversion-policy.md does not carry the worked example 4.4488")
	}

	// The single-unit exceptions the document enumerates.
	for _, exception := range []string{"Celsius only", "cd/m2", "lux", "kg CO2e"} {
		if !strings.Contains(doc, exception) {
			t.Errorf("docs/conversion-policy.md does not name the %s exception", exception)
		}
	}
}
