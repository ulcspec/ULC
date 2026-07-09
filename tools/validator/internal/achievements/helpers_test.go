package achievements

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot resolves the repository root. Tests run from
// tools/validator/internal/achievements; four levels up is the repo root.
func repoRoot(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return p
}

// rec wraps attestations in a minimal record.
func rec(atts ...map[string]any) map[string]any {
	arr := make([]any, len(atts))
	for i, a := range atts {
		arr[i] = a
	}
	return map[string]any{"attestations": arr}
}

// att builds an attestation entry from a program token plus optional extra fields.
func att(program string, extra map[string]any) map[string]any {
	m := map[string]any{"program": program}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

// evidence returns a well-formed source_document_ref (a FileReference with a 64-hex sha256).
func evidence() map[string]any {
	return map[string]any{"filename": "cert.pdf", "sha256": strings.Repeat("a", 64)}
}

// metric builds a sustainability_metric payload.
func metric(fields map[string]any) map[string]any {
	return fields
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// exampleFiles returns the paths of all example records.
func exampleFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(repoRoot(t), "examples", "*.ulc"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no example records found")
	}
	return matches
}

// exampleByName returns the path of a single example record.
func exampleByName(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "examples", name)
}

// loadRecord reads a .ulc file into a generic map.
func loadRecord(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return m
}

// deepCopyRecord returns an independent copy via a JSON round-trip.
func deepCopyRecord(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal for copy: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal for copy: %v", err)
	}
	return out
}

// loadTaxonomyEnum loads a taxonomy enum $def's members as a set.
func loadTaxonomyEnum(t *testing.T, def string) map[string]bool {
	t.Helper()
	path := filepath.Join(repoRoot(t), "schema", "taxonomy.schema.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read taxonomy: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse taxonomy: %v", err)
	}
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("taxonomy has no $defs")
	}
	d, ok := defs[def].(map[string]any)
	if !ok {
		t.Fatalf("taxonomy has no $def %q", def)
	}
	arr, ok := d["enum"].([]any)
	if !ok {
		t.Fatalf("$def %q is not an enum", def)
	}
	out := map[string]bool{}
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out[s] = true
		}
	}
	return out
}
