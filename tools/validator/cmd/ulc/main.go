// Command ulc is the reference validator and index builder for the ULC
// (Universal Luminaire Cutsheet) specification.
//
// Subcommands:
//
//	ulc validate <record.ulc>        Validate a record against the ULC schema.
//	ulc build-index <record.ulc>     Regenerate the record's index block.
//	ulc from-sheet <bundle-dir>      Convert a CSV workbook bundle into records.
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
	"github.com/ulcspec/ULC/tools/validator/internal/grade"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
	"github.com/ulcspec/ULC/tools/validator/internal/sheet"
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
	case "from-sheet":
		os.Exit(runFromSheet(args))
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
    from-sheet    Convert a CSV workbook bundle into validated ULC records.
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
  2. Builder parity (stored index matches the deterministic projection,
     including the computed index.conformance_level)
  3. Source-file SHA-256 hash verification (when files are reachable locally)
  4. Conformance report (INFO: the computed level plus guidance toward the next)

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

	// 4. Conformance report. The achieved level was already computed by the
	// builder and stored in index.conformance_level, and the parity step above
	// guards that stored value. This step is the human-facing report: it prints
	// the computed level plus guidance toward the next level (INFO only, never a
	// defect). A record is whatever level its data achieves; there is nothing to
	// fall short of, so conformance produces no WARNINGs.
	grade.Report(recordMap, report)

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

// --- from-sheet ---

// runFromSheet converts a CSV workbook bundle into validated ULC records. For
// each assembled record it builds the index (which stamps conformance_level),
// checks the required index keys, writes <out>/<record_id>.ulc.json, runs the
// schema validator plus the conformance report, and prints a one-line summary.
// It exits non-zero if any record fails schema validation.
func runFromSheet(args []string) int {
	fs := flag.NewFlagSet("from-sheet", flag.ExitOnError)
	var outDir, assetsDir string
	var allowMissing bool
	fs.StringVar(&outDir, "out", ".", "Directory to write <record_id>.ulc.json files into.")
	fs.StringVar(&assetsDir, "assets", "", "Directory referenced files (cutsheet, IES, attestation docs) resolve against. Defaults to the bundle directory.")
	fs.BoolVar(&allowMissing, "allow-missing-files", false, "Stamp the 64-zero sentinel SHA-256 and warn (instead of erroring) when a referenced file is absent on disk.")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `ulc from-sheet -- convert a CSV workbook bundle into validated ULC records.

Reads a directory of <sheet>.csv files (records.csv plus the related sheets),
assembles each record's deep blocks, computes dual-unit companions, SHA-256
hashes, and default provenance, then builds the index (stamping the conformance
level) and validates each record against the ULC schema.

All four authoring patterns are supported end to end: A (single-SKU) and C
(per-IES with derived provenance) as fixed-axes pins, B (CCT multiplier table)
and D (per-foot linear scaling) with generated declared_by_cct / declared_by_length
photometry tables.

Exit codes:
  0   every record assembled, built, and passed schema validation
  1   a conversion, build, or schema-validation error occurred
  2   usage error

USAGE
    ulc from-sheet <bundle-dir> [--out DIR] [--assets DIR] [--allow-missing-files]
`)
	}
	_ = fs.Parse(reorderFlagsFirst(args))
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	bundleDir := fs.Arg(0)

	results, err := sheet.Convert(bundleDir, sheet.Options{
		AssetsRoot:        assetsDir,
		AllowMissingFiles: allowMissing,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ulc from-sheet: %v\n", err)
		return 1
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "ulc from-sheet: create out dir %s: %v\n", outDir, err)
		return 1
	}

	// Build the validator once and reuse it across records. Prefer an in-repo
	// schema directory; fall back to the embedded schemas for released binaries.
	var v *validate.Validator
	if dir, ferr := validate.FindSchemaDir("", bundleDir); ferr == nil {
		v, err = validate.NewValidator(dir)
	} else {
		v, err = validate.NewValidatorEmbedded()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ulc from-sheet: %v\n", err)
		return 1
	}

	failed := false
	for _, res := range results {
		for _, w := range res.Warnings {
			fmt.Fprintf(os.Stderr, "warning: %s: %s\n", res.RecordID, w)
		}

		built := index.Build(res.Record)
		if missing := index.MissingRequiredKeys(built); len(missing) > 0 {
			fmt.Fprintf(os.Stderr, "ulc from-sheet: %s: builder cannot derive required index keys:\n", res.RecordID)
			for _, key := range missing {
				src := index.RequiredKeySources[key]
				if src == "" {
					src = "(always-present marker)"
				}
				fmt.Fprintf(os.Stderr, "  - %s  (populate %s)\n", key, src)
			}
			failed = true
			continue
		}
		res.Record["index"] = built

		outPath := filepath.Join(outDir, res.RecordID+".ulc.json")
		if err := writeRecord(outPath, res.Record); err != nil {
			fmt.Fprintf(os.Stderr, "ulc from-sheet: %v\n", err)
			failed = true
			continue
		}

		// Validate the assembled record. The validator wants the json.Number-typed
		// tree, but the in-memory record carries Go-native numbers; re-read the
		// file we just wrote through the strict decoder so the schema check sees
		// the same shape the validate subcommand does.
		report := findings.NewReport()
		data, rerr := os.ReadFile(outPath)
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "ulc from-sheet: re-read %s: %v\n", outPath, rerr)
			failed = true
			continue
		}
		rawTree, derr := decodeStrict(data)
		if derr != nil {
			fmt.Fprintf(os.Stderr, "ulc from-sheet: parse %s: %v\n", outPath, derr)
			failed = true
			continue
		}
		v.Validate(rawTree, report)
		validate.VerifyHashes(outDir, res.Record, report)
		grade.Report(res.Record, report)
		report.Finalize()

		level, _ := built["conformance_level"].(string)
		if level == "" {
			level = "none"
		}
		fmt.Printf("%s -> %s (%d findings)\n", res.RecordID, level, len(report.Findings))
		if report.HasErrors() {
			if err := report.WriteText(os.Stderr, outPath); err != nil {
				fmt.Fprintf(os.Stderr, "ulc from-sheet: write report: %v\n", err)
			}
			failed = true
		}
	}

	if failed {
		return 1
	}
	return 0
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

// valueFlags are the flag names across all subcommands that take a
// space-separated value, so reorderFlagsFirst keeps the value riding with its
// flag when partitioning. Boolean flags are absent from this set.
var valueFlags = map[string]bool{
	"-schema-dir": true, "--schema-dir": true,
	"-out": true, "--out": true,
	"-assets": true, "--assets": true,
}

// reorderFlagsFirst lets users write either `ulc sub <file> --check` or
// `ulc sub --check <file>`. Go's stdlib flag.Parse stops at the first
// non-flag arg, which would otherwise force flag-first ordering. Most of our
// flags are boolean; the value-taking ones are tracked in valueFlags so their
// following value rides with them through the partition.
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
			// A value-taking flag passed without an `=` consumes the next arg as
			// its value, which must ride with it.
			if valueFlags[a] {
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
