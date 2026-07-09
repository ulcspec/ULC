package completeness

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/achievements"
	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/validate"
)

// updateGolden regenerates the committed compat baselines instead of comparing.
// Run: go test ./internal/completeness -run TestGoldenCompat -update-golden
var updateGolden = flag.Bool("update-golden", false, "rewrite the golden compat baselines under testdata/golden")

// TestGoldenCompat is the release's no-change oracle. For every examples/*.ulc it
// runs the in-process validate pipeline exactly as cmd/ulc's runValidate does,
// minus the two machine-varying steps (builder parity and source-file hash
// verification): schema Validate on the raw json.Number tree, then
// completeness.Report on the normalized tree, then Finalize, then WriteText and
// WriteJSON into buffers. Each buffer is byte-compared against
// testdata/golden/<name>.{txt,json}. Any grading or rendering leak into an
// existing record turns this red, which is the whole point: v0.10.0 must not
// change any existing record's output.
//
// Determinism pins (v0.10.0 plan Phase A.1):
//   - recordPath passed to WriteText is the BASE filename. WriteText embeds it
//     verbatim (it does not basename), so passing the base name keeps the header
//     and footer host-independent.
//   - Verbose and OmitFlagHint are false (the validate-path defaults).
//   - validate.VerifyHashes is NOT called, so the machine-varying
//     source-file/not-found-locally INFO findings stay out of the buffers.
//
// The test globs examples/*.ulc, so the three v0.10.0 exit-sign records
// self-register once promoted in Phase E; their pairs are captured at promotion.
func TestGoldenCompat(t *testing.T) {
	root := repoRoot(t)
	v, err := validate.NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("new validator: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(root, "examples", "*.ulc"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no examples/*.ulc found")
	}
	sort.Strings(matches)

	goldenDir := filepath.Join("testdata", "golden")
	if *updateGolden {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", goldenDir, err)
		}
	}

	for _, path := range matches {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			gotTxt, gotJSON := renderGolden(t, v, path, name)
			// The baselines are rendered report output, not ULC records, so
			// their filenames drop the .ulc suffix. Keeping it would collide
			// with the record-index parity guard's *.ulc / *.ulc.json glob
			// (pre-commit hook and CI), which would try to build-index a
			// report as if it were a record. The bytes are unchanged: name is
			// still passed to renderGolden, so the record path embedded in the
			// text baseline stays verbatim.
			stem := strings.TrimSuffix(name, ".ulc")
			txtPath := filepath.Join(goldenDir, stem+".txt")
			jsonPath := filepath.Join(goldenDir, stem+".json")
			if *updateGolden {
				writeGolden(t, txtPath, gotTxt)
				writeGolden(t, jsonPath, gotJSON)
				return
			}
			compareGolden(t, txtPath, gotTxt)
			compareGolden(t, jsonPath, gotJSON)
		})
	}
}

// renderGolden runs the schema + completeness half of runValidate in process and
// returns the WriteText and WriteJSON bytes. recordPath is pinned to name (the
// base filename) so the WriteText header and footer are host-independent.
func renderGolden(t *testing.T, v *validate.Validator, path, name string) (txt, js []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	// Verbose=false, OmitFlagHint=false: the validate-path defaults.
	report := findings.NewReport()
	// 1. Schema validation on the untouched json.Number tree, matching runValidate.
	v.Validate(raw, report)
	// 2. Normalize numbers, then grade the normalized tree (json.Number closures
	// read only float64/int64). normalizeForTest mirrors main.normalizeNumbers.
	normalized, err := normalizeForTest(raw)
	if err != nil {
		t.Fatalf("normalize %s: %v", path, err)
	}
	rec, ok := normalized.(map[string]any)
	if !ok {
		t.Fatalf("%s: top-level JSON is not an object", path)
	}
	Report(rec, report)
	// The Product Achievements axis, emitted exactly as runValidate does (immediately
	// after completeness.Report), so the goldens capture the second axis too.
	achievements.Report(rec, report)
	report.Finalize()

	var txtBuf, jsBuf bytes.Buffer
	if err := report.WriteText(&txtBuf, name); err != nil {
		t.Fatalf("WriteText %s: %v", name, err)
	}
	if err := report.WriteJSON(&jsBuf); err != nil {
		t.Fatalf("WriteJSON %s: %v", name, err)
	}
	return txtBuf.Bytes(), jsBuf.Bytes()
}

func writeGolden(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write golden %s: %v", path, err)
	}
}

func compareGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (regenerate with: go test ./internal/completeness -run TestGoldenCompat -update-golden)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden mismatch for %s (regenerate with -update-golden if this change is intended):\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}
