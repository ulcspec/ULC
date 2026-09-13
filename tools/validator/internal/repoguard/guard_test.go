package repoguard

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFindRetiredRecordNamesNormalizesTrackedText(t *testing.T) {
	token := retiredRecordToken()
	finished := strings.Join([]string{".", "ulc"}, "")
	escaped := strings.Join([]string{"pattern \\.", "ulc", "(\\.", "json)?$"}, "")
	broken := strings.Join([]string{"record .u", "[l]", "c:.", "(json)"}, "")
	lines := []Line{
		{Path: "plain.md", Number: 2, Text: "record " + token},
		{Path: "hook", Number: 4, Text: escaped},
		{Path: "broken.txt", Number: 6, Text: broken},
		{Path: "finished.md", Number: 8, Text: "record " + finished},
		{Path: "history.md", Number: 10, Text: "record " + token},
	}

	matches := FindRetiredRecordNames(lines, map[string]bool{"history.md": true})
	if len(matches) != 3 {
		t.Fatalf("matches = %d, want 3: %#v", len(matches), matches)
	}
	for i, want := range lines[:3] {
		if matches[i] != want {
			t.Errorf("match %d = %#v, want original %#v", i, matches[i], want)
		}
	}
}

func TestScanTrackedTreeHasNoRetiredRecordName(t *testing.T) {
	result, err := ScanTrackedTree(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TrackedFiles) == 0 {
		t.Fatal("tracked file count is zero")
	}
	foundModule := false
	for _, path := range result.TrackedFiles {
		if path == "go.mod" {
			foundModule = true
			break
		}
	}
	if !foundModule {
		t.Error("tracked files do not include go.mod")
	}
	for _, match := range result.Matches {
		t.Errorf("%s:%d: retired record name in %q", match.Path, match.Number, match.Text)
	}
}

func TestScanTrackedTreeRequiresRepositoryRoot(t *testing.T) {
	start := filepath.Join(t.TempDir(), "outside")
	if _, err := ScanTrackedTree(start); err == nil {
		t.Fatal("ScanTrackedTree succeeded without a go.mod ancestor")
	}
}
