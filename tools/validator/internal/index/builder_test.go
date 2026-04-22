package index

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestBuilderMatchesStoredIndex is the load-bearing parity test for the Go
// port. Each canonical reference record in examples/ carries a stored `index`
// block that was produced by the Python builder; Build() must reproduce it
// exactly for every record, or the two implementations have drifted.
//
// Drift here is a hard failure. The spec's single-authority rule depends on
// both the Python and Go builders agreeing until the Python script retires.
func TestBuilderMatchesStoredIndex(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	examplesDir := filepath.Join(repoRoot, "examples")
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("read examples dir: %v", err)
	}
	var recordFiles []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".ulc" {
			continue
		}
		recordFiles = append(recordFiles, filepath.Join(examplesDir, e.Name()))
	}
	if len(recordFiles) == 0 {
		t.Fatalf("no .ulc records found under %s", examplesDir)
	}

	for _, path := range recordFiles {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			record, err := loadRecord(path)
			if err != nil {
				t.Fatalf("load %s: %v", path, err)
			}
			built := Build(record)
			stored, ok := record["index"].(map[string]any)
			if !ok {
				t.Fatalf("%s: record has no `index` block", path)
			}
			if missing := MissingRequiredKeys(built); len(missing) > 0 {
				t.Fatalf("%s: builder missing required keys: %v", path, missing)
			}
			if diffs := Diff(stored, built); len(diffs) > 0 {
				t.Errorf("%s: builder output diverges from stored index:\n%s",
					path, joinLines(diffs))
			}
		})
	}
}

func TestBUGFormatter(t *testing.T) {
	cases := []struct {
		in   map[string]any
		want string
	}{
		{map[string]any{"b": int64(1), "u": int64(0), "g": int64(2)}, "B1-U0-G2"},
		{map[string]any{"b": float64(3), "u": float64(2), "g": float64(4)}, "B3-U2-G4"},
		{map[string]any{"b": int64(1), "u": int64(0)}, ""}, // missing g
		{nil, ""},
	}
	for i, c := range cases {
		got := formatBUG(c.in)
		if got != c.want {
			t.Errorf("case %d: formatBUG(%v) = %q, want %q", i, c.in, got, c.want)
		}
	}
}

func TestMissingRequiredKeysSorted(t *testing.T) {
	built := Index{
		"x-ulc-generated":  true,
		"builder_version":  BuilderVersion,
		"catalog_model":    "FOO",
		"primary_category": "downlight",
	}
	missing := MissingRequiredKeys(built)
	want := []string{"manufacturer_slug", "nominal_input_power_w", "nominal_total_lumens"}
	if len(missing) != len(want) {
		t.Fatalf("got %v, want %v", missing, want)
	}
	for i := range want {
		if missing[i] != want[i] {
			t.Fatalf("got %v, want %v", missing, want)
		}
	}
}

// --- helpers ---

func loadRecord(path string) (Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	normalized, err := normalizeForTest(raw)
	if err != nil {
		return nil, err
	}
	m, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("top-level is not an object")
	}
	return m, nil
}

func normalizeForTest(v any) (any, error) {
	switch n := v.(type) {
	case map[string]any:
		for k, child := range n {
			fixed, err := normalizeForTest(child)
			if err != nil {
				return nil, err
			}
			n[k] = fixed
		}
		return n, nil
	case []any:
		for i, child := range n {
			fixed, err := normalizeForTest(child)
			if err != nil {
				return nil, err
			}
			n[i] = fixed
		}
		return n, nil
	case json.Number:
		s := n.String()
		isInt := true
		for _, r := range s {
			if r == '.' || r == 'e' || r == 'E' {
				isInt = false
				break
			}
		}
		if isInt {
			if i, err := n.Int64(); err == nil {
				return i, nil
			}
		}
		f, err := n.Float64()
		if err != nil {
			return nil, err
		}
		return f, nil
	default:
		return v, nil
	}
}

func joinLines(xs []string) string {
	out := ""
	for _, x := range xs {
		out += x + "\n"
	}
	return out
}
