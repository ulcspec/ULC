package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	embedded "github.com/ulcspec/ULC/schema"
)

// TestMethodologySourceDocumentTableIsTotal keeps the published source-document
// table synchronized with the closed SourceFileType vocabulary. Each token must
// appear in the table's token column exactly once, so a new type cannot ship
// undocumented and a retired type cannot linger in the guidance.
func TestMethodologySourceDocumentTableIsTotal(t *testing.T) {
	var taxonomy map[string]any
	if err := json.Unmarshal(embedded.TaxonomySchemaJSON, &taxonomy); err != nil {
		t.Fatalf("parse taxonomy schema: %v", err)
	}
	defs, _ := taxonomy["$defs"].(map[string]any)
	sourceFileType, _ := defs["SourceFileType"].(map[string]any)
	rawTokens, _ := sourceFileType["enum"].([]any)
	if len(rawTokens) == 0 {
		t.Fatal("SourceFileType has no enum tokens")
	}
	want := make(map[string]bool, len(rawTokens))
	for _, raw := range rawTokens {
		token, ok := raw.(string)
		if !ok {
			t.Fatalf("SourceFileType contains a non-string token %v", raw)
		}
		want[token] = true
	}

	path := filepath.Join(repoRoot(t), "docs", "methodology.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read methodology: %v", err)
	}
	text := string(data)
	start := strings.Index(text, "| Document | SourceFileType token | What it carries |")
	end := strings.Index(text, "\n### Standards corpus reviewed")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("methodology source-document table markers not found in order")
	}
	tokenPattern := regexp.MustCompile("`([a-z0-9_]+)`")
	seen := map[string]int{}
	for _, line := range strings.Split(text[start:end], "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 4 {
			continue
		}
		for _, match := range tokenPattern.FindAllStringSubmatch(cells[2], -1) {
			seen[match[1]]++
		}
	}

	for token := range want {
		if seen[token] != 1 {
			t.Errorf("SourceFileType token %q appears %d times in the methodology table, want exactly 1", token, seen[token])
		}
	}
	for token, count := range seen {
		if !want[token] {
			t.Errorf("methodology table token %q appears %d times but is not in SourceFileType", token, count)
		}
	}
}
