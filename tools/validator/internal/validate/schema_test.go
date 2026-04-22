package validate

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// TestValidatorAcceptsExampleRecords is the load-bearing positive test: each
// canonical reference record must validate clean against the schema, with no
// ERROR-level findings. A failure here means either the schema or the
// validator wiring (including cross-file $ref resolution) has regressed.
func TestValidatorAcceptsExampleRecords(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	examples := filepath.Join(root, "examples")
	matches, err := filepath.Glob(filepath.Join(examples, "*.ulc"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no .ulc files under %s", examples)
	}
	for _, path := range matches {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			doc := loadOrFail(t, path)
			report := findings.NewReport()
			v.Validate(doc, report)
			if report.HasErrors() {
				for _, f := range report.Findings {
					t.Logf("%s: %s", f.Level, f.Message)
				}
				t.Fatalf("%s: expected zero schema errors, got %d", path, len(report.Findings))
			}
		})
	}
}

// TestValidatorRejectsBrokenRecord asserts the validator catches a violation
// introduced to one of the canonical records (e.g., wiping a required field).
// This guards against silently-accepting-everything bugs in the wiring.
func TestValidatorRejectsBrokenRecord(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	// Start from a known-good record, then delete a required top-level field
	// (ulc_version) to force a schema violation.
	doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
	m, ok := doc.(map[string]any)
	if !ok {
		t.Fatalf("record is not an object")
	}
	delete(m, "ulc_version")

	report := findings.NewReport()
	v.Validate(m, report)
	if !report.HasErrors() {
		t.Fatalf("expected at least one schema error after deleting ulc_version, got none")
	}
	// Every finding at this stage should be a schema-violation code.
	for _, f := range report.Findings {
		if f.Code != findings.CodeSchemaViolation {
			t.Errorf("unexpected finding code %q, want %q", f.Code, findings.CodeSchemaViolation)
		}
	}
}

func TestFindSchemaDirExplicit(t *testing.T) {
	root := repoRoot(t)
	schemaDir := filepath.Join(root, "schema")
	got, err := FindSchemaDir(schemaDir, "")
	if err != nil {
		t.Fatalf("FindSchemaDir explicit: %v", err)
	}
	if got != schemaDir {
		t.Fatalf("got %q, want %q", got, schemaDir)
	}
}

func TestFindSchemaDirWalksUp(t *testing.T) {
	root := repoRoot(t)
	recordPath := filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc")
	got, err := FindSchemaDir("", recordPath)
	if err != nil {
		t.Fatalf("FindSchemaDir walk-up: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join(root, "schema")) && got != filepath.Join(root, "schema") {
		t.Fatalf("got %q, want suffix schema", got)
	}
}

// --- helpers ---

func repoRoot(t *testing.T) string {
	t.Helper()
	// Tests run from tools/validator/internal/validate; four levels up is repo root.
	p, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return p
}

func loadOrFail(t *testing.T, path string) any {
	t.Helper()
	data, err := readFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return doc
}

// readFile wraps os.ReadFile so the import surface of this test file stays
// narrow (helpful for scanning in a code review).
func readFile(path string) ([]byte, error) {
	return osReadFile(path)
}
