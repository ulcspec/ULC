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
		path := jsonPath("source_files", i)
		verifyOne(recordDir, m, path, report)
	}
}

func verifyOne(recordDir string, entry map[string]any, path string, report *findings.Report) {
	filename := asString(entry["filename"])
	declared := strings.ToLower(asString(entry["sha256"]))
	if filename == "" || declared == "" {
		// The JSON Schema enforces both fields as required on SourceFile, so
		// any missing value will already have been flagged as a schema error.
		// Skip here to avoid duplicate reporting.
		return
	}
	resolved := filename
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(recordDir, filename)
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
