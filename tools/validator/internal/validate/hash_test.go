package validate

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// TestVerifyHashesAllOutcomes covers the three paths the hash guard can take
// per entry: (a) the file exists and SHA-256 matches → silent pass,
// (b) the file exists and SHA-256 mismatches → ERROR, (c) the file does not
// exist locally → INFO (can't verify but not an error).
//
// Records always wrap integrity fields under source_files[].reference, so the
// fixtures here use the real schema shape; a regression where the verifier
// reads the wrong path would fail this test loudly.
func TestVerifyHashesAllOutcomes(t *testing.T) {
	tmp := t.TempDir()

	// (a) file exists and matches
	matchName := "matches.pdf"
	matchContent := []byte("hello ulc")
	writeFile(t, filepath.Join(tmp, matchName), matchContent)
	matchSum := sha256Hex(matchContent)

	// (b) file exists, declared hash is wrong
	mismatchName := "mismatch.ies"
	writeFile(t, filepath.Join(tmp, mismatchName), []byte("some bytes"))
	mismatchDeclared := sha256Hex([]byte("different bytes"))

	// (c) file not on disk — declared hash for something that does not exist
	missingName := "missing.ldt"
	missingDeclared := sha256Hex([]byte("irrelevant"))

	record := map[string]any{
		"source_files": []any{
			map[string]any{
				"file_type": "datasheet_pdf",
				"reference": map[string]any{
					"filename": matchName,
					"sha256":   matchSum,
				},
			},
			map[string]any{
				"file_type": "ies_file",
				"reference": map[string]any{
					"filename": mismatchName,
					"sha256":   mismatchDeclared,
				},
			},
			map[string]any{
				"file_type": "ldt_file",
				"reference": map[string]any{
					"filename": missingName,
					"sha256":   missingDeclared,
				},
			},
		},
	}

	report := findings.NewReport()
	VerifyHashes(tmp, record, report)

	var (
		errs  []findings.Finding
		warns []findings.Finding
		infos []findings.Finding
	)
	for _, f := range report.Findings {
		switch f.Level {
		case findings.LevelError:
			errs = append(errs, f)
		case findings.LevelWarning:
			warns = append(warns, f)
		case findings.LevelInfo:
			infos = append(infos, f)
		}
	}

	if len(errs) != 1 {
		t.Fatalf("expected 1 ERROR, got %d (%v)", len(errs), errs)
	}
	if errs[0].Code != findings.CodeSourceFileHashMismatch {
		t.Errorf("error code = %q, want %q", errs[0].Code, findings.CodeSourceFileHashMismatch)
	}
	if !strings.Contains(errs[0].Path, "/source_files/1/reference") {
		t.Errorf("error path = %q, want /source_files/1/reference", errs[0].Path)
	}

	if len(infos) != 1 {
		t.Fatalf("expected 1 INFO, got %d (%v)", len(infos), infos)
	}
	if infos[0].Code != findings.CodeSourceFileNotFound {
		t.Errorf("info code = %q, want %q", infos[0].Code, findings.CodeSourceFileNotFound)
	}
	if !strings.Contains(infos[0].Path, "/source_files/2/reference") {
		t.Errorf("info path = %q, want /source_files/2/reference", infos[0].Path)
	}

	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

// TestVerifyHashesRefusesAbsolutePath confirms the verifier refuses to hash
// files referenced by absolute path. `ulc validate` runs in CI on records
// supplied by pull requests; hashing arbitrary readable files on the runner
// would leak file fingerprints.
func TestVerifyHashesRefusesAbsolutePath(t *testing.T) {
	tmp := t.TempDir()

	// A real sensitive-looking path does not need to exist on disk — the guard
	// refuses absolute paths before the filesystem is touched.
	record := map[string]any{
		"source_files": []any{
			map[string]any{
				"file_type": "datasheet_pdf",
				"reference": map[string]any{
					"filename": "/etc/passwd",
					"sha256":   strings.Repeat("0", 64),
				},
			},
		},
	}
	report := findings.NewReport()
	VerifyHashes(tmp, record, report)

	if len(report.Findings) != 1 {
		t.Fatalf("expected exactly 1 finding, got %d (%v)", len(report.Findings), report.Findings)
	}
	got := report.Findings[0]
	if got.Level != findings.LevelInfo {
		t.Errorf("level = %s, want %s", got.Level, findings.LevelInfo)
	}
	if got.Code != findings.CodeSourceFileNotFound {
		t.Errorf("code = %s, want %s (any non-error is fine; absolute paths are not ERRORs)",
			got.Code, findings.CodeSourceFileNotFound)
	}
	if !strings.Contains(got.Message, "absolute") {
		t.Errorf("expected message to mention 'absolute', got %q", got.Message)
	}
}

// TestVerifyHashesRefusesTraversal confirms the verifier refuses relative
// paths that escape the record's directory via `../`, which is the same
// fingerprint-leak vector as absolute paths.
func TestVerifyHashesRefusesTraversal(t *testing.T) {
	outer := t.TempDir()
	recordDir := filepath.Join(outer, "records")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a file outside recordDir; the attacker's record tries to point
	// at it via `../secret.txt`.
	secretPath := filepath.Join(outer, "secret.txt")
	writeFile(t, secretPath, []byte("top secret"))

	record := map[string]any{
		"source_files": []any{
			map[string]any{
				"file_type": "datasheet_pdf",
				"reference": map[string]any{
					"filename": "../secret.txt",
					"sha256":   strings.Repeat("0", 64),
				},
			},
		},
	}
	report := findings.NewReport()
	VerifyHashes(recordDir, record, report)

	if len(report.Findings) != 1 {
		t.Fatalf("expected exactly 1 finding, got %d (%v)", len(report.Findings), report.Findings)
	}
	got := report.Findings[0]
	if got.Level != findings.LevelInfo {
		t.Errorf("level = %s, want %s", got.Level, findings.LevelInfo)
	}
	if !strings.Contains(got.Message, "outside the record directory") {
		t.Errorf("expected message to mention 'outside the record directory', got %q", got.Message)
	}
}

// TestVerifyHashesAllowsNestedPath confirms relative paths to subdirectories
// under the record's directory still work normally.
func TestVerifyHashesAllowsNestedPath(t *testing.T) {
	recordDir := t.TempDir()
	sub := filepath.Join(recordDir, "assets")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := []byte("nested source")
	writeFile(t, filepath.Join(sub, "ies.dat"), content)

	record := map[string]any{
		"source_files": []any{
			map[string]any{
				"file_type": "ies_file",
				"reference": map[string]any{
					"filename": "assets/ies.dat",
					"sha256":   sha256Hex(content),
				},
			},
		},
	}
	report := findings.NewReport()
	VerifyHashes(recordDir, record, report)

	if len(report.Findings) != 0 {
		t.Fatalf("expected 0 findings for valid nested path, got %d (%v)",
			len(report.Findings), report.Findings)
	}
}

// TestVerifyHashesSkipsFlatShape confirms the verifier silently skips entries
// that do not have the schema-required `reference` wrapper object. Those
// entries will already have been flagged as schema errors elsewhere in the
// validate pipeline, so double-reporting is noise.
func TestVerifyHashesSkipsFlatShape(t *testing.T) {
	record := map[string]any{
		"source_files": []any{
			map[string]any{
				"file_type": "datasheet_pdf",
				// No `reference` wrapper — simulate a malformed / old-shape record.
				"filename": "legacy.pdf",
				"sha256":   strings.Repeat("0", 64),
			},
		},
	}
	report := findings.NewReport()
	VerifyHashes("/tmp", record, report)
	if len(report.Findings) != 0 {
		t.Fatalf("expected no hash findings for malformed entry, got %v", report.Findings)
	}
}

// --- helpers ---

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
