// Command ulc is the reference validator and index builder for the ULC
// (Universal Luminaire Cutsheet) specification.
//
// Subcommands:
//
//	ulc validate <record.ulc>        Validate a record against the ULC schema.
//	ulc build-index <record.ulc>     Regenerate the record's index block.
//	ulc version                      Print the CLI version.
//
// For per-subcommand flags and semantics, run `ulc <subcommand> -h`.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
	"github.com/ulcspec/ULC/tools/validator/internal/validate"
)

// CLIVersion is the shipped binary version. Distinct from index.BuilderVersion
// (the projection-logic semver) so we can rev the CLI without bumping every
// previously-built index block. Overridden at release time by goreleaser via
// -ldflags -X main.CLIVersion=<tag>.
var CLIVersion = "0.4.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	sub, args := os.Args[1], os.Args[2:]
	switch sub {
	case "validate":
		os.Exit(runValidate(args))
	case "build-index":
		os.Exit(runBuildIndex(args))
	case "version", "-v", "--version":
		fmt.Printf("ulc %s (builder %s)\n", CLIVersion, index.BuilderVersion)
	case "help", "-h", "--help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "ulc: unknown subcommand %q\n\n", sub)
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `ulc -- reference validator and index builder for the ULC spec

USAGE
    ulc <subcommand> [options] <record.ulc>

SUBCOMMANDS
    validate      Validate a ULC record against the ULC schema.
    build-index   Regenerate the record's index block from its deep blocks.
    version       Print the CLI version.
    help          Print this help message.

Run 'ulc <subcommand> -h' for per-subcommand options.
`)
}

// --- validate ---

func runValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	var jsonOut bool
	var schemaDir string
	fs.BoolVar(&jsonOut, "json", false, "Emit findings as machine-readable JSON instead of human-readable text.")
	fs.StringVar(&schemaDir, "schema-dir", "", "Directory containing ulc.schema.json and taxonomy.schema.json. Auto-detected when omitted.")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `ulc validate -- validate a ULC record against the ULC schema.

Runs four checks and emits a findings report:
  1. JSON Schema Draft 2020-12 structural validation
  2. Builder parity (stored index matches the deterministic projection)
  3. Source-file SHA-256 hash verification (when files are reachable locally)
  4. Conformance grading (currently stubbed; full rubric lands post-pilot)

Exit codes:
  0   no ERROR findings (WARNING and INFO do not fail validation)
  1   at least one ERROR finding
  2   usage error

USAGE
    ulc validate [--json] [--schema-dir PATH] <record.ulc>
`)
	}
	_ = fs.Parse(reorderFlagsFirst(args))
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	recordPath := fs.Arg(0)

	data, err := os.ReadFile(recordPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ulc validate: read %s: %v\n", recordPath, err)
		return 1
	}
	// Strict single-value parse: UseNumber preserves JSON numbers for the
	// schema validator, and the EOF check rejects files that sneak
	// concatenated or trailing content past the default single-value semantics
	// of json.Decoder.
	rawTree, err := decodeStrict(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ulc validate: parse %s: %v\n", recordPath, err)
		return 1
	}

	// Build the validator. When --schema-dir is explicit, use that. When not
	// set and a schema/ directory is discoverable from the record's parents
	// or cwd, use it (matches in-repo dev workflow). Otherwise fall back to
	// the schemas embedded into the binary (released-binary workflow).
	var v *validate.Validator
	if schemaDir != "" {
		v, err = validate.NewValidator(schemaDir)
	} else if dir, ferr := validate.FindSchemaDir("", recordPath); ferr == nil {
		v, err = validate.NewValidator(dir)
	} else {
		v, err = validate.NewValidatorEmbedded()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ulc validate: %v\n", err)
		return 1
	}

	report := findings.NewReport()

	// 1. JSON Schema validation (on the json.Number-typed tree).
	v.Validate(rawTree, report)

	// Normalize numbers on the tree so the index builder sees int64 / float64
	// (matches Python's int/float dispatch). The validator has already seen
	// the untouched tree above, so this mutation is safe.
	normalized, err := normalizeNumbers(rawTree)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ulc validate: %v\n", err)
		return 1
	}
	recordMap, ok := normalized.(map[string]any)
	if !ok {
		fmt.Fprintf(os.Stderr, "ulc validate: %s: top-level JSON value is not an object\n", recordPath)
		return 1
	}

	// 2. Builder parity.
	built := index.Build(recordMap)
	if missing := index.MissingRequiredKeys(built); len(missing) > 0 {
		for _, key := range missing {
			src := index.RequiredKeySources[key]
			if src == "" {
				src = "(always-present marker)"
			}
			report.AddError(findings.CodeIndexBuilderMissingRequired, "/index/"+key,
				fmt.Sprintf("builder cannot derive required index key %s (populate %s)", key, src))
		}
	} else {
		stored, _ := recordMap["index"].(map[string]any)
		if stored == nil {
			stored = map[string]any{}
		}
		for _, diff := range index.Diff(stored, built) {
			// Diff strings already carry the key; reshape into a finding with
			// a JSON Pointer to the index property.
			trim := strings.TrimLeft(diff, " ")
			key, detail, _ := strings.Cut(trim, ":")
			report.AddError(findings.CodeIndexDrift, "/index/"+strings.TrimSpace(key),
				strings.TrimSpace(detail))
		}
	}

	// 3. Source-file hash verification (read files relative to the record).
	recordDir := filepath.Dir(recordPath)
	validate.VerifyHashes(recordDir, recordMap, report)

	// 4. Conformance grading stub. Full rubric deferred to a follow-up CLI
	// release informed by pilot feedback on what "standard" and "full" imply.
	if level, _ := recordMap["conformance_level"].(string); level != "" {
		report.AddInfo(findings.CodeConformanceGradingDeferred, "/conformance_level",
			fmt.Sprintf("conformance grading at level %q is not yet implemented; only structural, parity, and hash checks run", level))
	}

	report.Finalize()

	if jsonOut {
		if err := report.WriteJSON(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "ulc validate: write JSON: %v\n", err)
			return 1
		}
	} else {
		if err := report.WriteText(os.Stdout, recordPath); err != nil {
			fmt.Fprintf(os.Stderr, "ulc validate: write report: %v\n", err)
			return 1
		}
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}

// --- build-index ---

func runBuildIndex(args []string) int {
	fs := flag.NewFlagSet("build-index", flag.ExitOnError)
	var check, stdout bool
	fs.BoolVar(&check, "check", false, "Verify the record's stored index matches the builder output. Exits 1 on drift.")
	fs.BoolVar(&stdout, "stdout", false, "Print the built index to stdout without modifying the record.")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `ulc build-index -- regenerate a ULC record's index block.

The index block is a deterministic projection of the record's deep blocks. It
is forbidden by spec to hand-author the index. Default mode writes the computed
index back into the record in place.

USAGE
    ulc build-index <record.ulc>              # write in place
    ulc build-index <record.ulc> --stdout     # print built index, do not modify
    ulc build-index <record.ulc> --check      # verify stored index; exit 1 on drift
`)
	}
	_ = fs.Parse(reorderFlagsFirst(args))
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	if check && stdout {
		fmt.Fprintln(os.Stderr, "ulc build-index: --check and --stdout are mutually exclusive")
		return 2
	}
	recordPath := fs.Arg(0)
	record, err := readRecord(recordPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ulc build-index: %v\n", err)
		return 1
	}
	built := index.Build(record)
	if missing := index.MissingRequiredKeys(built); len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "ulc build-index: builder cannot derive required index keys for %s:\n", recordPath)
		for _, key := range missing {
			src := index.RequiredKeySources[key]
			if src == "" {
				src = "(always-present marker)"
			}
			fmt.Fprintf(os.Stderr, "  - %s  (populate %s)\n", key, src)
		}
		return 1
	}
	switch {
	case stdout:
		return printIndex(built)
	case check:
		stored, _ := record["index"].(map[string]any)
		if stored == nil {
			stored = map[string]any{}
		}
		if diffs := index.Diff(stored, built); len(diffs) > 0 {
			fmt.Fprintf(os.Stderr, "Index drift in %s:\n", recordPath)
			for _, d := range diffs {
				fmt.Fprintln(os.Stderr, d)
			}
			return 1
		}
		fmt.Printf("OK -- index in %s matches builder %s.\n", recordPath, index.BuilderVersion)
		return 0
	default:
		record["index"] = built
		if err := writeRecord(recordPath, record); err != nil {
			fmt.Fprintf(os.Stderr, "ulc build-index: %v\n", err)
			return 1
		}
		fmt.Printf("OK -- wrote index (builder %s) to %s.\n", index.BuilderVersion, recordPath)
		return 0
	}
}

func printIndex(idx index.Index) int {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(idx); err != nil {
		fmt.Fprintf(os.Stderr, "ulc build-index: %v\n", err)
		return 1
	}
	if _, err := os.Stdout.Write(buf.Bytes()); err != nil {
		return 1
	}
	return 0
}

// reorderFlagsFirst lets users write either `ulc sub <file> --check` or
// `ulc sub --check <file>`. Go's stdlib flag.Parse stops at the first
// non-flag arg, which would otherwise force flag-first ordering. All our
// flags are boolean (no space-separated values), so a simple partition works.
func reorderFlagsFirst(args []string) []string {
	flags := []string{}
	positional := []string{}
	skip := false
	for _, a := range args {
		if skip {
			flags = append(flags, a)
			skip = false
			continue
		}
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			// --schema-dir takes a value; if it was passed without an `=`,
			// the following arg is the value and must ride with it.
			if a == "-schema-dir" || a == "--schema-dir" {
				skip = true
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}

// --- I/O helpers ---

func readRecord(path string) (index.Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	raw, err := decodeStrict(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	record, err := normalizeNumbers(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	m, ok := record.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: top-level JSON value is not an object", path)
	}
	return m, nil
}

// decodeStrict parses exactly one JSON value from data. Returns an error when
// there is trailing non-whitespace content after the first value (a second
// JSON value, garbage, or an unclosed stream), so files like
// `{"valid": "json"}GARBAGE` are rejected instead of silently ignoring the
// tail the way json.Decoder's default single-value behavior would.
//
// UseNumber preserves JSON numbers as json.Number, matching what the schema
// validator expects and what normalizeNumbers consumes downstream.
func decodeStrict(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	var trailing any
	err := dec.Decode(&trailing)
	if err == nil {
		return nil, errors.New("trailing content after JSON value")
	}
	if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("trailing content after JSON value: %w", err)
	}
	return raw, nil
}

// normalizeNumbers walks a parsed JSON tree and converts json.Number values
// into int64 when they are integral (no decimal point, no exponent) and fit,
// float64 otherwise. This matches Python's json.loads dispatch so the builder
// produces identical output across both implementations.
func normalizeNumbers(v any) (any, error) {
	switch n := v.(type) {
	case map[string]any:
		for k, child := range n {
			fixed, err := normalizeNumbers(child)
			if err != nil {
				return nil, err
			}
			n[k] = fixed
		}
		return n, nil
	case []any:
		for i, child := range n {
			fixed, err := normalizeNumbers(child)
			if err != nil {
				return nil, err
			}
			n[i] = fixed
		}
		return n, nil
	case json.Number:
		return numberFromJSON(n)
	default:
		return v, nil
	}
}

func numberFromJSON(n json.Number) (any, error) {
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
		return nil, fmt.Errorf("invalid number %q: %w", s, err)
	}
	return f, nil
}

func writeRecord(path string, record index.Record) error {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(record); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
