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

// zeroPlaceholderSHA256 is the 64-zero placeholder the from-sheet converter
// stamps for a referenced file that is missing on disk under
// --allow-missing-files. It is schema-valid lowercase hex, so nothing
// structural distinguishes a draft from a published record; verifyOne flags
// it so the draft state stays visible at every reference site the run
// verifies (the attestation evidence sites run under --verify-evidence).
const zeroPlaceholderSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"

func verifyOne(recordDir string, ref map[string]any, path string, report *findings.Report) {
	filename := asString(ref["filename"])
	declared := strings.ToLower(asString(ref["sha256"]))
	if filename == "" || declared == "" {
		return
	}
	if declared == zeroPlaceholderSHA256 {
		report.AddWarning(findings.CodeSourceFilePlaceholderHash, path,
			fmt.Sprintf("declared SHA-256 for %s is the 64-zero placeholder stamped by draft conversion (from-sheet --allow-missing-files); replace it with the real file hash before publishing", filename))
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
	// First pass: lexical containment. Catches `../` escapes that are visible
	// in the declared path without touching the filesystem.
	rel, err := filepath.Rel(recordDirAbs, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		report.AddInfo(findings.CodeSourceFileNotFound, path,
			fmt.Sprintf("filename %q resolves outside the record directory; hash verification skipped", filename))
		return
	}
	// Second pass: symlink resolution. A lexically-clean path like
	// "plausible.pdf" can still be a symlink that points at /etc/passwd;
	// walking just the declared path would miss that. Resolve both sides
	// (the record directory and the target) so the containment check
	// reflects what os.Open will actually reach.
	resolvedReal, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			report.AddInfo(findings.CodeSourceFileNotFound, path,
				fmt.Sprintf("source file %s is not present locally; SHA-256 cannot be verified here", filename))
			return
		}
		report.AddWarning(findings.CodeSourceFileUnreadable, path,
			fmt.Sprintf("could not resolve symlinks for %s: %v", filename, err))
		return
	}
	recordDirReal, err := filepath.EvalSymlinks(recordDirAbs)
	if err != nil {
		report.AddWarning(findings.CodeSourceFileUnreadable, path,
			fmt.Sprintf("could not resolve symlinks for record directory %q: %v", recordDir, err))
		return
	}
	relReal, err := filepath.Rel(recordDirReal, resolvedReal)
	if err != nil || relReal == ".." || strings.HasPrefix(relReal, ".."+string(filepath.Separator)) {
		report.AddInfo(findings.CodeSourceFileNotFound, path,
			fmt.Sprintf("filename %q resolves outside the record directory after symlink resolution; hash verification skipped", filename))
		return
	}
	// Open the real (symlink-resolved) path so the hash covers what the
	// containment check approved, not whatever the declared path might
	// transit through.
	f, err := os.Open(resolvedReal)
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
