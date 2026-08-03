package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// updateScopeGolden regenerates the committed scope manifests instead of comparing.
// Run: go test ./cmd/ulc -run TestCLIScopeGoldens -update-scope-golden
var updateScopeGolden = flag.Bool("update-scope-golden", false, "rewrite the scope manifests under testdata/scope-golden")

// cliVersionSentinel replaces the live CLIVersion value on both sides of the
// golden comparison. CLIVersion is "0.4.0-dev" in a source build and is
// ldflag-overridden at release, so an un-normalized golden would be build-mode
// dependent and would fail against a release binary.
const cliVersionSentinel = "CLI_VERSION"

// scopeGoldenDir is where the per-example manifests live.
const scopeGoldenDir = "testdata/scope-golden"

// captureStderr runs fn with os.Stderr redirected to a pipe and returns what it
// wrote plus the exit code. The shipped captureStdout helper redirects stdout
// only, and the usage block goes to stderr.
func captureStderr(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	os.Stderr = w
	defer func() { os.Stderr = old }()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	code := fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	return <-done, code
}

// normalizeCLIVersion swaps the live cli_version value for the sentinel, leaving
// the field's presence and position untouched.
func normalizeCLIVersion(t *testing.T, out string) string {
	t.Helper()
	live := `"cli_version": ` + quoteJSON(t, CLIVersion)
	want := `"cli_version": ` + quoteJSON(t, cliVersionSentinel)
	if !strings.Contains(out, live) {
		t.Fatalf("output does not carry the live cli_version %q:\n%s", CLIVersion, out)
	}
	return strings.Replace(out, live, want, 1)
}

func quoteJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal %q: %v", s, err)
	}
	return string(b)
}

// scopeExampleNames lists every shipped example record by filename.
func scopeExampleNames(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(repoRoot(t), "examples", "*.ulc"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no example records found")
	}
	sort.Strings(matches)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, filepath.Base(m))
	}
	return out
}

// TestCLIScopeGoldens byte-compares every example's manifest against its
// committed golden (after normalizing cli_version) and asserts the command is
// deterministic across runs.
func TestCLIScopeGoldens(t *testing.T) {
	for _, name := range scopeExampleNames(t) {
		stem := strings.TrimSuffix(name, ".ulc")
		record := exampleRecord(t, name)

		out, code := captureStdout(t, func() int { return runScope([]string{record}) })
		if code != 0 {
			t.Fatalf("%s: exit = %d, want 0", name, code)
		}
		// Determinism: the same input yields byte-identical output.
		again, code2 := captureStdout(t, func() int { return runScope([]string{record}) })
		if code2 != 0 || again != out {
			t.Errorf("%s: second run is not byte-identical to the first", name)
		}

		// The field's presence and position are asserted independently of its value.
		var order []string
		dec := json.NewDecoder(strings.NewReader(out))
		if _, err := dec.Token(); err != nil { // opening brace
			t.Fatalf("%s: %v", name, err)
		}
		for dec.More() {
			tok, err := dec.Token()
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			key, ok := tok.(string)
			if !ok {
				t.Fatalf("%s: unexpected object key token %v", name, tok)
			}
			order = append(order, key)
			var discard json.RawMessage
			if err := dec.Decode(&discard); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		if len(order) < 2 || order[0] != "scope_version" || order[1] != "cli_version" {
			t.Errorf("%s: envelope key order = %v, want scope_version then cli_version first", name, order)
		}

		normalized := normalizeCLIVersion(t, out)
		goldenPath := filepath.Join(scopeGoldenDir, stem+".json")
		if *updateScopeGolden {
			if err := os.MkdirAll(scopeGoldenDir, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", scopeGoldenDir, err)
			}
			if err := os.WriteFile(goldenPath, []byte(normalized), 0o644); err != nil {
				t.Fatalf("write %s: %v", goldenPath, err)
			}
			t.Logf("wrote %s", goldenPath)
			continue
		}
		want, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("read %s (regenerate with -update-scope-golden): %v", goldenPath, err)
		}
		if normalized != string(want) {
			t.Errorf("%s: manifest does not match %s", name, goldenPath)
		}
	}
}

// TestCLIScopeUsageErrors pins the exit-2 cases. Every assertion is on the
// return code of the in-process call: runScope's FlagSet uses ContinueOnError, so
// the flag package never calls os.Exit inside the test binary.
func TestCLIScopeUsageErrors(t *testing.T) {
	vode := exampleRecord(t, vodeRecord)
	cases := []struct {
		name string
		args []string
	}{
		{"no positional args", []string{}},
		{"two positional args", []string{vode, vode}},
		{"unrecognized flag", []string{vode, "--json"}},
	}
	for _, c := range cases {
		c := c
		_, rc := captureStderr(t, func() int { return runScope(c.args) })
		if rc != 2 {
			t.Errorf("%s: exit = %d, want 2", c.name, rc)
		}
	}
}

// TestCLIScopeHelp asserts that -h exits 0, writes the usage block to stderr, and
// writes nothing to stdout.
func TestCLIScopeHelp(t *testing.T) {
	var errOut string
	stdout, rc := captureStdout(t, func() int {
		var inner int
		errOut, inner = captureStderr(t, func() int { return runScope([]string{"-h"}) })
		return inner
	})
	if rc != 0 {
		t.Errorf("-h exit = %d, want 0", rc)
	}
	if stdout != "" {
		t.Errorf("-h wrote %d bytes to stdout, want none:\n%s", len(stdout), stdout)
	}
	if !strings.Contains(errOut, "ulc scope -- print the grading-scope manifest") {
		t.Errorf("-h usage block missing from stderr:\n%s", errOut)
	}
	if strings.Count(errOut, "USAGE") != 1 {
		t.Errorf("-h printed the usage block %d times, want once:\n%s", strings.Count(errOut, "USAGE"), errOut)
	}
}

// TestCLIScopeReadErrors pins the exit-1 cases: nothing usable as a ULC record
// yields a message on stderr and no partial JSON on stdout.
func TestCLIScopeReadErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}
	cases := []struct {
		name string
		path string
	}{
		{"missing file", filepath.Join(dir, "does-not-exist.ulc")},
		{"trailing garbage", write("garbage.ulc", `{"record_id":"x"}GARBAGE`)},
		{"top-level array", write("array.ulc", `[{"record_id":"x"}]`)},
		{"unrepresentable number", write("bignum.ulc", `{"record_id":"x","n":1e400}`)},
	}
	for _, c := range cases {
		c := c
		var stderrOut string
		stdout, rc := captureStdout(t, func() int {
			var inner int
			stderrOut, inner = captureStderr(t, func() int { return runScope([]string{c.path}) })
			return inner
		})
		if rc != 1 {
			t.Errorf("%s: exit = %d, want 1", c.name, rc)
		}
		if stdout != "" {
			t.Errorf("%s: wrote %d bytes to stdout, want none:\n%s", c.name, len(stdout), stdout)
		}
		if !strings.HasPrefix(stderrOut, "ulc scope: ") {
			t.Errorf("%s: stderr = %q, want a `ulc scope: ` diagnostic", c.name, stderrOut)
		}
	}
}

// TestCLIScopeEnvelopeEchoes covers the two record-controlled envelope fields:
// they are omitted when absent or non-string, and a hostile string still yields
// parseable JSON.
func TestCLIScopeEnvelopeEchoes(t *testing.T) {
	dir := t.TempDir()
	run := func(t *testing.T, name, body string) map[string]any {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		out, rc := captureStdout(t, func() int { return runScope([]string{p}) })
		if rc != 0 {
			t.Fatalf("%s: exit = %d, want 0", name, rc)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("%s: output is not parseable JSON: %v\n%s", name, err, out)
		}
		return doc
	}

	t.Run("empty object omits both echoes", func(t *testing.T) {
		doc := run(t, "empty.ulc", `{}`)
		if _, ok := doc["record_id"]; ok {
			t.Error("record_id should be omitted when absent")
		}
		if _, ok := doc["ulc_version"]; ok {
			t.Error("ulc_version should be omitted when absent")
		}
		items, _ := doc["items"].([]any)
		if len(items) != 35 {
			t.Errorf("empty-object manifest has %d items, want 35", len(items))
		}
		blocks, _ := doc["blocks"].([]any)
		if len(blocks) != 12 {
			t.Errorf("empty-object manifest has %d blocks, want 12", len(blocks))
		}
	})

	t.Run("non-string echoes are omitted", func(t *testing.T) {
		doc := run(t, "nonstring.ulc", `{"record_id":7,"ulc_version":{"major":1}}`)
		if _, ok := doc["record_id"]; ok {
			t.Error("a numeric record_id should be omitted")
		}
		if _, ok := doc["ulc_version"]; ok {
			t.Error("an object ulc_version should be omitted")
		}
	})

	t.Run("hostile echoes stay parseable", func(t *testing.T) {
		// A script-closing tag, a quote, a backslash, a control character and a
		// lone surrogate escape (invalid UTF-8 once decoded).
		hostile := `</script>\" \\ \u0007 \ud800`
		doc := run(t, "hostile.ulc", `{"record_id":"`+hostile+`","ulc_version":"</b>"}`)
		got, _ := doc["record_id"].(string)
		if got == "" {
			t.Error("hostile record_id was dropped")
		}
		if !strings.Contains(got, "</script>") {
			t.Errorf("record_id = %q, want the hostile string echoed verbatim", got)
		}
	})
}
