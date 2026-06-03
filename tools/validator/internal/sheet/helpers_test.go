package sheet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// contains reports whether sub occurs in s; a small readability wrapper used in
// warning-message assertions.
func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

// readFixture returns the bytes of a file under testdata/bundle as a string.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "bundle", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

// writeFile writes content to path, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// writeFixtureCopy copies the full testdata/bundle (CSVs plus the cutsheet and
// IES files) into dir so a test can mutate one sheet in isolation.
func writeFixtureCopy(t *testing.T, dir string) {
	t.Helper()
	copyBundle(t, filepath.Join("testdata", "bundle"), dir)
}

// copyBundle copies every file in the src bundle directory into dst so a test can
// mutate one sheet in isolation while keeping the referenced asset files present.
func copyBundle(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read bundle dir %s: %v", src, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatalf("read bundle file %s: %v", e.Name(), err)
		}
		writeFile(t, filepath.Join(dst, e.Name()), string(data))
	}
}
