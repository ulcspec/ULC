package sheet

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
	"github.com/ulcspec/ULC/tools/validator/internal/validate"
)

// bundleWithColumns copies the reference bundle into a temp dir and appends the
// given header/value pairs to the single records row, so a test exercises new
// columns without restating the whole sheet.
func bundleWithColumns(t *testing.T, cols map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writeFixtureCopy(t, dir)

	path := filepath.Join(dir, "records.csv")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read records.csv: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected a header and one record row in the fixture, got %d lines", len(lines))
	}
	// Deterministic order: the sheet reader keys on the header, but a stable
	// column order keeps the generated fixture easy to compare.
	names := make([]string, 0, len(cols))
	for name := range cols {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		// The row is appended as raw text, so a value carrying a comma or a
		// quote would silently reshape the row rather than fail loudly.
		if strings.ContainsAny(cols[name], ",\"") {
			t.Fatalf("column %q value must not contain a comma or a quote: %q", name, cols[name])
		}
		lines[0] += "," + name
		lines[1] += "," + cols[name]
	}
	writeFile(t, path, strings.Join(lines, "\n")+"\n")
	return dir
}

func convertOneRecord(t *testing.T, dir string, opts Options) Result {
	t.Helper()
	results, err := Convert(dir, opts)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	return results[0]
}

func wantString(t *testing.T, rec map[string]any, path, want string) {
	t.Helper()
	v, ok := getPath(rec, path)
	if !ok {
		t.Errorf("%s is absent, want %q", path, want)
		return
	}
	got, ok := v.(string)
	if !ok {
		t.Errorf("%s = %T, want a string", path, v)
		return
	}
	if got != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}

// TestConvertLifecycleColumns asserts the five records-sheet lifecycle columns
// land at their schema paths: the four supersession members under
// superseded_by, and the top-level discontinued_at date.
func TestConvertLifecycleColumns(t *testing.T) {
	const successorHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	dir := bundleWithColumns(t, map[string]string{
		"superseded_by_record_id":      "acme-orbit-2400-4000k",
		"superseded_by_record_sha256":  successorHash,
		"superseded_by_catalog_number": "ORB-2400-40K-90",
		"superseded_by_catalog_model":  "Orbit Downlight 2400",
		"discontinued_at":              "2027-03-31",
		"record_status":                "superseded",
	})

	rec := convertOneRecord(t, dir, Options{}).Record
	wantString(t, rec, "superseded_by.record_id", "acme-orbit-2400-4000k")
	wantString(t, rec, "superseded_by.record_sha256", successorHash)
	wantString(t, rec, "superseded_by.catalog_number", "ORB-2400-40K-90")
	wantString(t, rec, "superseded_by.catalog_model", "Orbit Downlight 2400")
	wantString(t, rec, "discontinued_at", "2027-03-31")
	wantString(t, rec, "record_status", "superseded")

	// The columns must assemble a schema-valid record, the way the from-sheet
	// command validates every record before writing it.
	wantSchemaValid(t, rec)
}

// wantSchemaValid stamps the index and validates the assembled record against
// the live schema, the round trip the from-sheet command performs.
func wantSchemaValid(t *testing.T, rec map[string]any) {
	t.Helper()
	rec["index"] = index.Build(rec)
	v, err := validate.NewValidator(schemaDir(t))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	report := findings.NewReport()
	v.Validate(numberTree(t, rec), report)
	if report.HasErrors() {
		t.Errorf("assembled record must validate; got: %+v", report.Findings)
	}
}

// TestConvertWarrantyColumns asserts the warranty columns assemble the whole
// shared_warranty block: the integer term, the basis token, the free-text
// scope, and the conditions document hashed from the bundle with its revision
// overrides applied.
func TestConvertWarrantyColumns(t *testing.T) {
	const conditions = "acme-warranty-conditions.pdf"
	const body = "Acme limited warranty conditions, revision 4.\n"
	dir := bundleWithColumns(t, map[string]string{
		"warranty_term_years":                      "5",
		"warranty_term_basis":                      "shipment",
		"warranty_scope":                           "Luminaire and driver; battery pro-rata after year 2.",
		"warranty_conditions_file":                 conditions,
		"warranty_conditions_file__revision_label": "Rev 4",
		"warranty_conditions_file__revision_date":  "2026-02-10",
	})
	writeFile(t, filepath.Join(dir, conditions), body)

	rec := convertOneRecord(t, dir, Options{}).Record

	years, ok := getPath(rec, "product_family.shared_warranty.term_years")
	if !ok {
		t.Fatal("shared_warranty.term_years is absent")
	}
	// KindNumber emits an integral value as an int64 so the schema's integer
	// type is satisfied without a float round trip.
	switch n := years.(type) {
	case int64:
		if n != 5 {
			t.Errorf("term_years = %d, want 5", n)
		}
	case float64:
		t.Errorf("term_years = %v (float64), want an integral value", n)
	default:
		t.Errorf("term_years = %T, want an integer", years)
	}

	wantString(t, rec, "product_family.shared_warranty.term_basis", "shipment")
	wantString(t, rec, "product_family.shared_warranty.scope",
		"Luminaire and driver; battery pro-rata after year 2.")

	sum := sha256.Sum256([]byte(body))
	wantString(t, rec, "product_family.shared_warranty.conditions_document.filename", conditions)
	wantString(t, rec, "product_family.shared_warranty.conditions_document.sha256", hex.EncodeToString(sum[:]))
	wantString(t, rec, "product_family.shared_warranty.conditions_document.revision_label", "Rev 4")
	wantString(t, rec, "product_family.shared_warranty.conditions_document.revision_date", "2026-02-10")

	wantSchemaValid(t, rec)
}

// TestConvertWarrantyConditionsFileMissing pins the missing-file behavior the
// new path-input column inherits: the zero-value Options fail the conversion,
// and AllowMissingFiles stamps the zero sentinel and marks the record a draft.
func TestConvertWarrantyConditionsFileMissing(t *testing.T) {
	cols := map[string]string{"warranty_conditions_file": "not-in-the-bundle.pdf"}

	if _, err := Convert(bundleWithColumns(t, cols), Options{}); err == nil {
		t.Fatal("expected an error for an absent warranty conditions file")
	}

	res := convertOneRecord(t, bundleWithColumns(t, cols), Options{AllowMissingFiles: true})
	if !res.HasMissingFileSentinel {
		t.Error("expected HasMissingFileSentinel with AllowMissingFiles")
	}
	wantString(t, res.Record, "product_family.shared_warranty.conditions_document.sha256", zeroSHA256)
}

// TestTemplateHeaderCoversRecordColumns keeps the shipped workbook template and
// the column table coupled: nothing else fails when a template header is
// misspelled, which would silently disable the column for every author using
// the template. The check is one-directional by design, because the template
// legitimately carries headers the column table does not drive (record_id, the
// path-input columns, the applicability headers, and the extensions pair).
func TestTemplateHeaderCoversRecordColumns(t *testing.T) {
	path := filepath.Join(filepath.Dir(schemaDir(t)), "templates", "workbook", "records.csv")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open template: %v", err)
	}
	defer f.Close()
	header, err := csv.NewReader(f).Read()
	if err != nil {
		t.Fatalf("read template header: %v", err)
	}
	present := make(map[string]bool, len(header))
	for _, h := range header {
		h = strings.TrimSpace(h)
		// The reader is last-cell-wins on a repeated header, so a duplicate
		// silently discards whatever the author typed in the first column.
		if present[h] {
			t.Errorf("header %q appears twice in templates/workbook/records.csv", h)
		}
		present[h] = true
	}

	for _, c := range recordColumns {
		if !present[c.Header] {
			t.Errorf("column %q has no header in templates/workbook/records.csv", c.Header)
		}
	}
	// The records sheet's path-input columns and their revision overrides are
	// read by name in convert.go rather than through recordColumns, so the
	// loop above cannot see them.
	for _, literal := range []string{
		"cutsheet_file",
		"cutsheet_file__revision_label",
		"cutsheet_file__revision_date",
		"warranty_conditions_file",
		"warranty_conditions_file__revision_label",
		"warranty_conditions_file__revision_date",
	} {
		if !present[literal] {
			t.Errorf("path-input column %q has no header in templates/workbook/records.csv", literal)
		}
	}
}
