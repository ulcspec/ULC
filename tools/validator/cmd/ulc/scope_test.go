package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
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

// captureOutErr runs fn with both standard streams redirected and returns what it
// wrote to each plus the exit code.
func captureOutErr(t *testing.T, fn func() int) (stdout, stderr string, code int) {
	t.Helper()
	stdout, code = captureStdout(t, func() int {
		var inner int
		stderr, inner = captureStderr(t, fn)
		return inner
	})
	return stdout, stderr, code
}

// normalizeCLIVersion swaps the live cli_version value for the sentinel, leaving
// the field's presence and position untouched. Both values are a semver-ish tag
// or the fixed sentinel, so neither can contain a character this encoder escapes
// (which now includes <, > and &), and the tokens are built by concatenation.
func normalizeCLIVersion(t *testing.T, out string) string {
	t.Helper()
	live := `"cli_version": "` + CLIVersion + `"`
	want := `"cli_version": "` + cliVersionSentinel + `"`
	if !strings.Contains(out, live) {
		t.Fatalf("output does not carry the live cli_version %q:\n%s", CLIVersion, out)
	}
	return strings.Replace(out, live, want, 1)
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
	names := scopeExampleNames(t)
	expected := map[string]bool{}
	for _, name := range names {
		expected[strings.TrimSuffix(name, ".ulc")+".json"] = true
	}

	for _, name := range names {
		name := name
		// Subtests, so one missing golden reports itself instead of aborting the
		// other seven comparisons (and, under -update-scope-golden, instead of
		// leaving testdata/ half regenerated).
		t.Run(name, func(t *testing.T) {
			stem := strings.TrimSuffix(name, ".ulc")
			record := exampleRecord(t, name)

			out, code := captureStdout(t, func() int { return runScope([]string{record}) })
			if code != 0 {
				t.Fatalf("exit = %d, want 0", code)
			}
			// Determinism: the same input yields byte-identical output.
			again, code2 := captureStdout(t, func() int { return runScope([]string{record}) })
			if code2 != 0 || again != out {
				t.Errorf("second run is not byte-identical to the first")
			}

			// The envelope opens with the contract version, then the CLI version.
			// Asserted on the raw text so a regenerated golden cannot bless a
			// reordering of its own. This does NOT pin the scope_version literal
			// (wantPrefix is built from the same constant); TestCLIScopeContractVersion
			// does that.
			wantPrefix := "{\n  \"scope_version\": \"" + scopeContractVersion + "\",\n  \"cli_version\": \""
			if !strings.HasPrefix(out, wantPrefix) {
				t.Errorf("envelope must open with scope_version %q then cli_version; got:\n%s",
					scopeContractVersion, firstLines(out, 4))
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
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read %s (regenerate with -update-scope-golden): %v", goldenPath, err)
			}
			if normalized != string(want) {
				t.Errorf("manifest does not match %s.\n%s", goldenPath, lineDiff(string(want), normalized))
			}
		})
	}

	// An orphan golden means an example was renamed or removed and its manifest
	// was left behind, where it would silently stop being compared to anything.
	if *updateScopeGolden {
		return
	}
	entries, err := os.ReadDir(scopeGoldenDir)
	if err != nil {
		t.Fatalf("read %s: %v", scopeGoldenDir, err)
	}
	for _, e := range entries {
		if !e.IsDir() && !expected[e.Name()] {
			t.Errorf("%s/%s has no matching record in examples/; delete it or restore the record", scopeGoldenDir, e.Name())
		}
	}
}

// firstLines returns at most n leading lines of s, for compact failure output.
func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// lineDiff reports the first differing line and the surrounding counts, so a
// golden mismatch in CI names what moved instead of only that something did.
// The goldens run to hundreds of lines, so a full dump is not useful.
func lineDiff(want, got string) string {
	w := strings.Split(want, "\n")
	g := strings.Split(got, "\n")
	for i := 0; i < len(w) && i < len(g); i++ {
		if w[i] != g[i] {
			return fmt.Sprintf("first difference at line %d:\n  want: %s\n   got: %s\n(want %d lines, got %d)",
				i+1, w[i], g[i], len(w), len(g))
		}
	}
	return fmt.Sprintf("one side is a prefix of the other (want %d lines, got %d)", len(w), len(g))
}

// TestCLIScopeUsageErrors pins the exit-2 cases. Every assertion is on the
// return code of the in-process call: runScope's FlagSet uses ContinueOnError, so
// the flag package never calls os.Exit inside the test binary.
func TestCLIScopeUsageErrors(t *testing.T) {
	vode := exampleRecord(t, vodeRecord)
	cases := []struct {
		name    string
		args    []string
		wantErr string // a distinguishing fragment of the stderr diagnostic
	}{
		{"no positional args", []string{}, "USAGE"},
		{"two positional args", []string{vode, vode}, "USAGE"},
		{"unrecognized flag", []string{vode, "--json"}, "not defined"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, rc := captureOutErr(t, func() int { return runScope(c.args) })
			if rc != 2 {
				t.Errorf("exit = %d, want 2", rc)
			}
			if stdout != "" {
				t.Errorf("wrote %d bytes to stdout, want none:\n%s", len(stdout), stdout)
			}
			// Without this the three cases are indistinguishable and any of them
			// could be reaching exit 2 by the wrong route.
			if !strings.Contains(stderr, c.wantErr) {
				t.Errorf("stderr does not contain %q:\n%s", c.wantErr, stderr)
			}
			// The parse-error arm must not re-print the block Parse already printed.
			if n := strings.Count(stderr, "USAGE"); n != 1 {
				t.Errorf("usage block printed %d times, want once:\n%s", n, stderr)
			}
		})
	}
}

// TestCLIScopeFlagAfterPositional pins the reorderFlagsFirst routing. Without it
// the stdlib parser stops at the first non-flag argument, so `ulc scope rec -h`
// would exit 2 instead of printing help, diverging from every sibling
// subcommand. (Dropping the reorder also makes the unrecognized-flag case in
// TestCLIScopeUsageErrors reach exit 2 by the wrong route, failing its stderr
// assertion; this test is the direct witness, that one is collateral.)
func TestCLIScopeFlagAfterPositional(t *testing.T) {
	vode := exampleRecord(t, vodeRecord)
	stdout, stderr, rc := captureOutErr(t, func() int { return runScope([]string{vode, "-h"}) })
	if rc != 0 {
		t.Errorf("`ulc scope <record> -h` exit = %d, want 0", rc)
	}
	if stdout != "" {
		t.Errorf("help wrote %d bytes to stdout, want none", len(stdout))
	}
	if !strings.Contains(stderr, "ulc scope -- print the grading-scope manifest") {
		t.Errorf("help block missing from stderr:\n%s", stderr)
	}
}

// TestCLIUsageListsScope pins the subcommand's entry in the top-level usage
// block, which is the only discovery surface for it.
func TestCLIUsageListsScope(t *testing.T) {
	buf := &bytes.Buffer{}
	usage(buf)
	if !strings.Contains(buf.String(), "scope") {
		t.Errorf("top-level usage block does not list the scope subcommand:\n%s", buf.String())
	}
}

// TestCLIScopeHelp asserts that -h exits 0, writes the usage block to stderr, and
// writes nothing to stdout.
func TestCLIScopeHelp(t *testing.T) {
	stdout, errOut, rc := captureOutErr(t, func() int { return runScope([]string{"-h"}) })
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
		t.Run(c.name, func(t *testing.T) {
			stdout, stderrOut, rc := captureOutErr(t, func() int { return runScope([]string{c.path}) })
			if rc != 1 {
				t.Errorf("exit = %d, want 1", rc)
			}
			if stdout != "" {
				t.Errorf("wrote %d bytes to stdout, want none:\n%s", len(stdout), stdout)
			}
			if !strings.HasPrefix(stderrOut, "ulc scope: ") {
				t.Errorf("stderr = %q, want a `ulc scope: ` diagnostic", stderrOut)
			}
		})
	}
}

// TestCLIScopeEnvelopeEchoes covers the two record-controlled envelope fields:
// they are omitted when absent, non-string, or empty, and a hostile string is
// escaped on the wire while still round-tripping through a JSON parser.
func TestCLIScopeEnvelopeEchoes(t *testing.T) {
	dir := t.TempDir()
	run := func(t *testing.T, name string, body []byte) (map[string]any, string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, body, 0o644); err != nil {
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
		return doc, out
	}

	t.Run("empty object omits both echoes", func(t *testing.T) {
		// Item and block counts are pinned in internal/completeness, which owns
		// the rubric; the subject here is echo omission and array shape alone.
		doc, _ := run(t, "empty.ulc", []byte(`{}`))
		if _, ok := doc["record_id"]; ok {
			t.Error("record_id should be omitted when absent")
		}
		if _, ok := doc["ulc_version"]; ok {
			t.Error("ulc_version should be omitted when absent")
		}
		if _, ok := doc["items"].([]any); !ok {
			t.Error("items must always be an array, never null")
		}
		if _, ok := doc["blocks"].([]any); !ok {
			t.Error("blocks must always be an array, never null")
		}
	})

	t.Run("non-string echoes are omitted", func(t *testing.T) {
		doc, _ := run(t, "nonstring.ulc", []byte(`{"record_id":7,"ulc_version":{"major":1}}`))
		if _, ok := doc["record_id"]; ok {
			t.Error("a numeric record_id should be omitted")
		}
		if _, ok := doc["ulc_version"]; ok {
			t.Error("an object ulc_version should be omitted")
		}
	})

	t.Run("empty-string echoes are omitted", func(t *testing.T) {
		doc, _ := run(t, "emptystring.ulc", []byte(`{"record_id":"","ulc_version":""}`))
		if _, ok := doc["record_id"]; ok {
			t.Error("an empty record_id should be omitted")
		}
		if _, ok := doc["ulc_version"]; ok {
			t.Error("an empty ulc_version should be omitted")
		}
	})

	t.Run("hostile echoes are escaped on the wire", func(t *testing.T) {
		// A script-closing tag, an escaped quote, a backslash, a control
		// character, and raw 0x80-range bytes. Note the raw bytes do NOT reach
		// the encoder as invalid UTF-8: encoding/json coerces them to U+FFFD
		// while DECODING, exactly as it would a \ud800 escape. They are kept
		// because they prove that path is safe end to end; the assertions that
		// carry this subtest are the wire-level escaping checks below.
		body := []byte(`{"record_id":"</script>\" \\ \u0007 `)
		body = append(body, 0x80, 0xfe, 0xff)
		body = append(body, []byte(`","ulc_version":"</b>&x"}`)...)

		doc, raw := run(t, "hostile.ulc", body)
		got, _ := doc["record_id"].(string)
		if got == "" {
			t.Fatal("hostile record_id was dropped")
		}
		// It still round-trips to what a consumer's JSON parser sees.
		for _, want := range []string{"</script>", `"`, `\`, "\x07"} {
			if !strings.Contains(got, want) {
				t.Errorf("decoded record_id %q is missing %q", got, want)
			}
		}
		// On the WIRE the angle brackets and ampersand must be escaped, so a
		// consumer inlining the manifest into a page cannot be broken out of.
		// Asserting on the decoded value alone would pass either way, which is
		// what made the previous version of this test unable to see the setting.
		for _, forbidden := range []string{"</script>", "<", ">", "&"} {
			if strings.Contains(raw, forbidden) {
				t.Errorf("raw manifest contains unescaped %q; HTML escaping must stay on:\n%s", forbidden, raw)
			}
		}
		if !strings.Contains(raw, `\u003c`) {
			t.Errorf("raw manifest does not carry the escaped form of '<':\n%s", raw)
		}
	})
}

// TestCLIScopeContractVersion pins the manifest contract's version literal. The
// goldens carry it too, but they are regenerable by their own flag, so without a
// hand-maintained assertion a bump could ship unnoticed.
func TestCLIScopeContractVersion(t *testing.T) {
	if scopeContractVersion != "1.0.0" {
		t.Errorf("scopeContractVersion = %q, want %q. Bumping it is a contract change: "+
			"update the README bullet and the CHANGELOG alongside this constant.",
			scopeContractVersion, "1.0.0")
	}
}

// TestPrintJSONEscapeModes pins BOTH sides of printJSON's escapeHTML switch, at
// the call sites that choose it. The switch is shared by build-index and scope,
// so testing only the scope direction would let a flip of the build-index call
// site pass silently and change that subcommand's output for any record carrying
// <, > or &, diverging --stdout from the in-place record write.
func TestPrintJSONEscapeModes(t *testing.T) {
	// A record whose index will carry an ampersand and angle brackets.
	rec := filepath.Join(t.TempDir(), "amp.ulc")
	body := `{
  "ulc_version": "1.0.0",
  "record_id": "amp-test",
  "record_status": "draft",
  "product_family": {
    "manufacturer": {"slug": "amp", "display_name": "A & B <Lighting>"},
    "catalog_model": "X<1> & Y"
  }
}`
	if err := os.WriteFile(rec, []byte(body), 0o644); err != nil {
		t.Fatalf("write record: %v", err)
	}

	t.Run("build-index --stdout does not escape", func(t *testing.T) {
		out, code := captureStdout(t, func() int { return runBuildIndex([]string{rec, "--stdout"}) })
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.Contains(out, "X<1> & Y") {
			t.Errorf("build-index --stdout must leave <, > and & literal to preserve the record's byte shape; got:\n%s", out)
		}
		if strings.Contains(out, `\u0026`) || strings.Contains(out, `\u003c`) {
			t.Errorf("build-index --stdout must not escape HTML characters; got:\n%s", out)
		}
	})

	t.Run("scope escapes", func(t *testing.T) {
		out, code := captureStdout(t, func() int { return runScope([]string{rec}) })
		if code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		// record_id is a plain slug here, so assert on the whole document: no raw
		// angle bracket or ampersand may appear anywhere in a manifest.
		for _, forbidden := range []string{"<", ">", "&"} {
			if strings.Contains(out, forbidden) {
				t.Errorf("scope manifest contains unescaped %q:\n%s", forbidden, out)
			}
		}
	})
}
