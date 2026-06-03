package sheet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// zeroSHA256 is the 64-zero sentinel stamped into a reference when a referenced
// file is missing on disk and --allow-missing-files is set. It mirrors the
// sentinel the ULC templates use for not-yet-hashed references, and it is a
// schema-valid lowercase-hex SHA-256 string so the record still validates while
// signaling "hash not computed".
const zeroSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"

// fileHasher resolves path-input columns against an assets root and computes
// their SHA-256. allowMissing controls the missing-file behavior: a hard error
// by default, or the zero sentinel plus a warning when set.
type fileHasher struct {
	assetsRoot   string
	allowMissing bool
	// warnings accumulates the human-facing messages emitted when a missing file
	// is tolerated under allowMissing. The command surfaces these to the user.
	warnings []string
	// sentinelStamped is true once a zero-sentinel SHA-256 has been stamped for a
	// missing file under allowMissing. Callers read this structured flag (rather
	// than parsing warning text) to mark the record a DRAFT.
	sentinelStamped bool
}

// hashFile resolves filename against the assets root, reads it, and returns the
// lowercase-hex SHA-256. When the file is missing: returns an error unless
// allowMissing is set, in which case it returns the zero sentinel and records a
// warning. Any other read error is always fatal.
func (h *fileHasher) hashFile(filename string) (string, error) {
	// Referenced files must be relative paths under the assets root. Rejecting
	// absolute paths and ".." traversal keeps records portable (no local
	// filesystem paths leak into emitted FileReference filenames) and prevents
	// the converter from hashing files outside the assets directory.
	if filepath.IsAbs(filename) {
		return "", fmt.Errorf("referenced file %q must be a relative path under the assets root, not an absolute path", filename)
	}
	cleaned := filepath.Clean(filename)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("referenced file %q escapes the assets root via .. traversal; reference files by their relative path under the assets directory", filename)
	}
	resolved := filepath.Join(h.assetsRoot, cleaned)
	f, err := os.Open(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			if h.allowMissing {
				h.sentinelStamped = true
				h.warnings = append(h.warnings,
					fmt.Sprintf("file %q not found under assets root %q: stamping zero-sentinel SHA-256 (--allow-missing-files)", filename, h.assetsRoot))
				return zeroSHA256, nil
			}
			return "", fmt.Errorf("referenced file %q not found under assets root %q: cannot compute SHA-256 (pass --allow-missing-files to stamp the zero sentinel instead)", filename, h.assetsRoot)
		}
		return "", fmt.Errorf("open referenced file %q: %w", filename, err)
	}
	defer f.Close()

	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", fmt.Errorf("read referenced file %q: %w", filename, err)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}
