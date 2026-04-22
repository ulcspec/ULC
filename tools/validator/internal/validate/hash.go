package validate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// VerifyHashes iterates the record's declared source_files and checks every
// file whose local path is resolvable from recordDir. A missing file emits
// INFO (the user may not have the source files locally); a file whose SHA-256
// does not match emits ERROR.
//
// recordDir is the directory the record lives in, used to resolve relative
// `filename` entries. Absolute paths are honored as-is.
//
// Each source_files entry wraps the integrity fields inside a `reference`
// object:
//
//	"source_files": [
//	  { "file_type": "datasheet_pdf",
//	    "reference": { "filename": "...", "sha256": "..." } }
//	]
//
// The schema enforces that wrapper shape (`$defs/SourceFile` requires
// `reference`), so missing `reference` or missing nested fields will already
// be flagged as schema errors and are not re-reported here.
func VerifyHashes(recordDir string, record map[string]any, report *findings.Report) {
	arr, ok := record["source_files"].([]any)
	if !ok {
		return
	}
	for i, entry := range arr {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		ref, ok := m["reference"].(map[string]any)
		if !ok {
			continue
		}
		path := jsonPath("source_files", i) + "/reference"
		verifyOne(recordDir, ref, path, report)
	}
}

func verifyOne(recordDir string, ref map[string]any, path string, report *findings.Report) {
	filename := asString(ref["filename"])
	declared := strings.ToLower(asString(ref["sha256"]))
	if filename == "" || declared == "" {
		return
	}
	// Constrain hash verification to files at-or-below the record's directory.
	// `ulc validate` runs in CI on PR-provided records, so honoring absolute
	// paths or `../` traversal would let a crafted record cause the runner to
	// open arbitrary readable files and report their SHA-256 in the findings
	// output — a fingerprint-leak vector.
	if filepath.IsAbs(filename) {
		report.AddInfo(findings.CodeSourceFileNotFound, path,
			fmt.Sprintf("filename %q is absolute; hash verification only runs against files under the record's directory", filename))
		return
	}
	recordDirAbs, err := filepath.Abs(recordDir)
	if err != nil {
		report.AddWarning(findings.CodeSourceFileUnreadable, path,
			fmt.Sprintf("could not resolve record directory %q: %v", recordDir, err))
		return
	}
	resolved, err := filepath.Abs(filepath.Join(recordDirAbs, filename))
	if err != nil {
		report.AddWarning(findings.CodeSourceFileUnreadable, path,
			fmt.Sprintf("could not resolve filename %q: %v", filename, err))
		return
	}
	rel, err := filepath.Rel(recordDirAbs, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		report.AddInfo(findings.CodeSourceFileNotFound, path,
			fmt.Sprintf("filename %q resolves outside the record directory; hash verification skipped", filename))
		return
	}
	f, err := os.Open(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			report.AddInfo(findings.CodeSourceFileNotFound, path,
				fmt.Sprintf("source file %s is not present locally; SHA-256 cannot be verified here", filename))
			return
		}
		report.AddWarning(findings.CodeSourceFileUnreadable, path,
			fmt.Sprintf("could not open source file %s: %v", filename, err))
		return
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		report.AddWarning(findings.CodeSourceFileUnreadable, path,
			fmt.Sprintf("could not read source file %s: %v", filename, err))
		return
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != declared {
		report.AddError(findings.CodeSourceFileHashMismatch, path,
			fmt.Sprintf("SHA-256 mismatch for %s: declared %s, computed %s", filename, declared, got))
	}
}

// jsonPath builds a simple JSON Pointer for array element access.
func jsonPath(root string, idx int) string {
	return fmt.Sprintf("/%s/%d", root, idx)
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
