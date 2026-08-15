// Command ulc is the reference validator and index builder for the ULC
// (Universal Luminaire Cutsheet) specification.
//
// Subcommands:
//
//	ulc validate <record.ulc>        Validate a record against the ULC schema.
//	ulc build-index <record.ulc>     Regenerate the record's index block.
//	ulc from-sheet <input>           Convert a workbook (CSV bundle or .xlsx) into records.
//	ulc scope <record.ulc>           Print the record's grading-scope manifest.
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
	"regexp"
	"strings"
	"time"

	"github.com/ulcspec/ULC/tools/validator/internal/achievements"
	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
	"github.com/ulcspec/ULC/tools/validator/internal/findings"
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
	case "scope":
		os.Exit(runScope(args))
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
    from-sheet    Convert a workbook (CSV bundle or .xlsx) into validated ULC records.
    scope         Print the grading-scope manifest for a ULC record.
    version       Print the CLI version.
    help          Print this help message.

Run 'ulc <subcommand> -h' for per-subcommand options.
`)
}

// --- validate ---

func runValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	var jsonOut bool
	var verbose bool
	var schemaDir string
	var expiry bool
	var asOf string
	var expiryWindow int
	var verifyEvidence bool
	fs.BoolVar(&jsonOut, "json", false, "Emit findings as machine-readable JSON instead of human-readable text.")
	fs.BoolVar(&verbose, "verbose", false, "Include the optional conformance and achievement findings (the enrichment roadmap, observation notes, and per-theme achievement state and roadmap) in text output. JSON always includes them.")
	fs.StringVar(&schemaDir, "schema-dir", "", "Directory containing ulc.schema.json and taxonomy.schema.json. Auto-detected when omitted.")
	fs.BoolVar(&expiry, "expiry", false, "Opt in to the advisory attestation-expiry check. Advisory: never changes the exit code or the computed index.")
	fs.StringVar(&asOf, "as-of", "", "Evaluation date for --expiry as YYYY-MM-DD (default: today). Requires --expiry.")
	fs.IntVar(&expiryWindow, "expiry-window", 90, "Days ahead to flag upcoming expiry for --expiry (0..36500). Requires --expiry.")
	fs.BoolVar(&verifyEvidence, "verify-evidence", false, "Also byte-verify attestation evidence documents (source_document_ref). Absent files stay INFO; a hash mismatch is an ERROR.")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `ulc validate -- validate a ULC record against the ULC schema.

Runs the following checks and emits a findings report:
  1. JSON Schema Draft 2020-12 validation (structure and declared value formats)
  2. Builder parity (stored index matches the deterministic projection,
     including the computed index.conformance_level)
  3. File-reference SHA-256 hash verification (when files are reachable
     locally): source_files entries, the family cutsheet, the emergency
     photometry reference, the family warranty-conditions document, and
     media manifest entries
  4. Conformance report (INFO: the computed grade plus a per-grade roadmap to full)
  5. Product Achievements report (INFO: the per-theme achievement summary; the
     per-theme state and roadmap show under --verbose or --json)

  Always reported: a declared hash equal to the 64-zero draft placeholder
  is flagged with a warning, whether or not the file is reachable locally.

Optional expiry advisory (opt in with --expiry):
  --expiry            Preview attestation and declaration expiry against a date.
                      Advisory: never changes the exit code or the computed index.
  --as-of DATE        Evaluation date as YYYY-MM-DD (default: today). Requires --expiry.
  --expiry-window N   Days ahead to flag upcoming expiry, 0..36500 (default: 90).
                      Requires --expiry.

Optional evidence verification (opt in with --verify-evidence):
  --verify-evidence   Also byte-verify attestation evidence documents
                      (attestations and product_family.shared_attestations
                      source_document_ref entries). A locally absent document
                      stays INFO; a document whose SHA-256 does not match is
                      an ERROR and fails validation.

Exit codes:
  0   no ERROR findings (WARNING and INFO do not fail validation)
  1   at least one ERROR finding
  2   usage error

USAGE
    ulc validate [--json] [--verbose] [--schema-dir PATH] [--verify-evidence]
                 [--expiry [--as-of DATE] [--expiry-window N]] <record.ulc>
`)
	}
	_ = fs.Parse(reorderFlagsFirst(args))
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	recordPath := fs.Arg(0)

	// The expiry flags are meaningful only with --expiry. Resolve the as-of default and
	// validate the flag values here so a usage error exits 2 before any work is done. The
	// wall clock is read here and nowhere below the CLI, only when --as-of is absent.
	setFlags := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })
	if (setFlags["as-of"] || setFlags["expiry-window"]) && !expiry {
		fmt.Fprintln(os.Stderr, "ulc validate: --as-of and --expiry-window require --expiry")
		return 2
	}
	if expiry {
		if asOf == "" {
			asOf = time.Now().Format("2006-01-02")
		} else if _, perr := time.Parse("2006-01-02", asOf); perr != nil {
			fmt.Fprintf(os.Stderr, "ulc validate: --as-of %q is not a valid YYYY-MM-DD date\n", asOf)
			return 2
		}
		if expiryWindow < 0 || expiryWindow > 36500 {
			fmt.Fprintf(os.Stderr, "ulc validate: --expiry-window %d out of range (0..36500)\n", expiryWindow)
			return 2
		}
	}

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
	// --verbose surfaces the conformance observation findings in text output; JSON
	// always carries them. Set before rendering (it only affects WriteText).
	report.Verbose = verbose

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

	// 3. File-reference hash verification (read files relative to the record).
	recordDir := filepath.Dir(recordPath)
	validate.VerifyFileReferences(recordDir, recordMap, validate.VerifyOptions{Evidence: verifyEvidence}, report)

	// 4. Conformance report. The achieved grade was already computed by the
	// builder and stored in index.conformance_level, and the parity step above
	// guards that stored value. This step is the human-facing report: it prints
	// the computed grade plus a per-grade roadmap to full (INFO only, never a
	// defect). A record is whatever level its data achieves; there is nothing to
	// fall short of, so conformance produces no WARNINGs.
	completeness.Report(recordMap, report)

	// 5. Product Achievements report: the second, orthogonal grading axis. Emits the
	// achievements headline plus the verbose-only per-theme states and roadmap. Not
	// gated on the conformance level (the axes are independent).
	achievements.Report(recordMap, report)

	// 6. Optional expiry advisory, opt in with --expiry. Gated at add time (only emitted
	// when the flag is set), so a default run's findings, footer, and exit code are
	// untouched. WARNING findings here never trip HasErrors, so the exit code stays advisory.
	if expiry {
		achievements.ReportExpiry(recordMap, asOf, expiryWindow, report)
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
				// Each line quotes the record's own stored index values, and
				// this subcommand runs no schema validation, so they are
				// arbitrary record-supplied strings.
				fmt.Fprintln(os.Stderr, findings.SanitizeText(d))
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

// recordIDSafe reports whether a record_id is usable as a filename component.
// It mirrors the schema's record_id slug pattern. The --out write needs no
// such guard because it happens only after schema validation has enforced the
// pattern; the --draft-out write happens BEFORE validation, and a workbook is
// untrusted input, so a draft filename must not be built from an unchecked
// cell (a record_id carrying a path separator or dot segments could otherwise
// write outside the draft directory).
var recordIDSafe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString

// fromSheetContractVersion is the version of the `ulc from-sheet --json`
// report's own output contract, mirroring the scope manifest's model: it is
// additive-only for its 1.x lifetime (new fields, new status values, and new
// arrays bump the minor; existing fields, key strings, and semantics do not
// change), and it is independent of both CLIVersion and the ULC schema
// version line, which the report is not part of.
const fromSheetContractVersion = "1.0.0"

// fromSheetRecordOut is one record's outcome in the --json report. record_id
// and pattern derive from workbook input; record_id is an unvalidated,
// workbook-controlled string and consumers must treat it as untrusted (never
// a filesystem path or a trust anchor), exactly as `ulc scope` documents for
// its own record_id echo. warnings is always present (possibly empty);
// findings carries the same {findings, summary} object `ulc validate --json`
// emits, present whenever validation ran for the record.
type fromSheetRecordOut struct {
	RecordID         string           `json:"record_id"`
	Pattern          string           `json:"pattern"`
	Status           string           `json:"status"`
	ConformanceLevel string           `json:"conformance_level,omitempty"`
	Path             string           `json:"path,omitempty"`
	DraftPath        string           `json:"draft_path,omitempty"`
	Warnings         []string         `json:"warnings"`
	MissingIndexKeys []string         `json:"missing_index_keys,omitempty"`
	Error            string           `json:"error,omitempty"`
	Findings         *findings.Report `json:"findings,omitempty"`
}

// fromSheetDoc is the --json report envelope.
type fromSheetDoc struct {
	FromSheetVersion string               `json:"from_sheet_version"`
	CLIVersion       string               `json:"cli_version"`
	Records          []fromSheetRecordOut `json:"records"`
	Summary          fromSheetSummary     `json:"summary"`
}

// fromSheetSummary counts outcomes across the run.
type fromSheetSummary struct {
	Written int `json:"written"`
	Drafts  int `json:"drafts"`
	Failed  int `json:"failed"`
}

// patternToken is the compact pattern identifier the --json contract carries.
// Pattern.String carries a parenthetical human label meant for error
// messages; using it here would couple the machine contract to display
// wording.
func patternToken(p sheet.Pattern) string {
	switch p {
	case sheet.PatternA:
		return "A"
	case sheet.PatternB:
		return "B"
	case sheet.PatternC:
		return "C"
	case sheet.PatternD:
		return "D"
	default:
		return "unknown"
	}
}

// runFromSheet converts a CSV bundle directory or a native .xlsx workbook into
// validated ULC records. For each assembled record it builds the index (which
// stamps conformance_level), checks the required index keys, writes
// <out>/<record_id>.ulc.json only after schema validation passes, runs the
// schema validator plus the conformance report, and prints a one-line summary.
// A record carrying placeholder hashes is a DRAFT: it is never written to
// --out, and --draft-out saves it as <record_id>.draft.json instead. --json
// replaces the summary lines with one machine-readable report on stdout. It
// exits non-zero if any record fails schema validation or any draft exists.
func runFromSheet(args []string) int {
	fs := flag.NewFlagSet("from-sheet", flag.ExitOnError)
	var outDir, assetsDir, draftDir string
	var allowMissing, jsonOut bool
	fs.StringVar(&outDir, "out", ".", "Directory to write <record_id>.ulc.json files into.")
	fs.StringVar(&assetsDir, "assets", "", "Directory referenced files (cutsheet, warranty conditions, IES, attestation docs) resolve against. Defaults to the bundle directory.")
	fs.BoolVar(&allowMissing, "allow-missing-files", false, "When a referenced file is absent on disk, stamp the 64-zero sentinel SHA-256 and treat the record as a DRAFT (reported, not written to --out; the run exits non-zero) instead of erroring immediately.")
	fs.StringVar(&draftDir, "draft-out", "", "Directory to write DRAFT records into as <record_id>.draft.json. Only meaningful with --allow-missing-files; drafts are never written to --out and the run still exits non-zero.")
	fs.BoolVar(&jsonOut, "json", false, "Emit one machine-readable JSON conversion report to stdout instead of the per-record summary lines.")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `ulc from-sheet -- convert a manufacturer workbook into validated ULC records.

Accepts either a CSV bundle (a directory of <sheet>.csv files: records.csv plus
the related sheets) or a native .xlsx workbook (one tab per sheet, named the same
way). Assembles each record's deep blocks, computes dual-unit companions,
SHA-256 hashes, and default provenance, then builds the index (stamping the
conformance level) and validates each record against the ULC schema.

Dual-unit fields accept either entry side: author the SI column (for example
overall_diameter_mm) or its Imperial companion column (overall_diameter_in),
never both for one field on one row. The authored value is written verbatim;
the companion leaf is computed per the published conversion policy
(docs/conversion-policy.md, shipped in the release archive as
CONVERSION-POLICY.md).

All four authoring patterns are supported end to end: A (single-SKU) and C
(per-IES with derived provenance) as fixed-axes pins, B (CCT multiplier table)
and D (per-foot linear scaling) with generated declared_by_cct / declared_by_length
photometry tables. Optional comprehensive sheets (alpha_opic, flicker_metrics,
lumen_maintenance_package, zonal_lumens, lcs_zonal_lumens, ingredient_list,
cie97_lmf / cie97_llmf) add full-level depth when present.

--json emits one JSON document on stdout: from_sheet_version (additive-only
contract, currently 1.0.0), cli_version, one records[] entry per converted
record (record_id, pattern, status written/draft/failed, conformance_level,
warnings, path or draft_path where a file was written, missing_index_keys
naming any required index keys the builder could not derive, error carrying
a short reason when a record failed outside schema validation, and findings
in the same shape ulc validate --json emits for records that reached
validation), and a summary rollup. Exit codes are unchanged. A workbook-level
conversion error (before records are assembled) produces no document; the
error goes to stderr.

Exit codes:
  0   every record assembled, built, and passed schema validation
  1   a conversion, build, or schema-validation error occurred
  2   usage error

USAGE
    ulc from-sheet <bundle-dir|workbook.xlsx> [--out DIR] [--assets DIR]
                   [--allow-missing-files [--draft-out DIR]] [--json]
`)
	}
	_ = fs.Parse(reorderFlagsFirst(args))
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	input := fs.Arg(0)

	results, err := sheet.Convert(input, sheet.Options{
		AssetsRoot:        assetsDir,
		AllowMissingFiles: allowMissing,
	})
	if err != nil {
		// The converter quotes workbook content (column headers, cell values)
		// into its errors, and the workbook is the untrusted input here.
		fmt.Fprintf(os.Stderr, "ulc from-sheet: %s\n", findings.SanitizeText(err.Error()))
		return 1
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "ulc from-sheet: create out dir %s: %v\n", outDir, err)
		return 1
	}
	// --draft-out is meaningful only with --allow-missing-files (only that
	// flag can produce a draft), so a bare --draft-out has no side effect.
	if draftDir != "" && allowMissing {
		if err := os.MkdirAll(draftDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "ulc from-sheet: create draft dir %s: %v\n", draftDir, err)
			return 1
		}
	}

	// Build the validator once and reuse it across records. Prefer an in-repo
	// schema directory; fall back to the embedded schemas for released binaries.
	var v *validate.Validator
	if dir, ferr := validate.FindSchemaDir("", input); ferr == nil {
		v, err = validate.NewValidator(dir)
	} else {
		v, err = validate.NewValidatorEmbedded()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ulc from-sheet: %v\n", err)
		return 1
	}

	doc := fromSheetDoc{FromSheetVersion: fromSheetContractVersion, CLIVersion: CLIVersion, Records: []fromSheetRecordOut{}}
	failed := false
	sawSentinel := false
	for _, res := range results {
		for _, w := range res.Warnings {
			fmt.Fprintf(os.Stderr, "warning: %s: %s\n", findings.SanitizeText(res.RecordID), findings.SanitizeText(w))
		}
		entry := fromSheetRecordOut{
			RecordID: res.RecordID,
			Pattern:  patternToken(res.Pattern),
			Warnings: append([]string{}, res.Warnings...),
		}
		recordSentinel := res.HasMissingFileSentinel
		if recordSentinel {
			sawSentinel = true
		}

		built := index.Build(res.Record)
		// The builder always stamps a conformance grade (it floors at
		// incomplete), so every entry carries one, even a record whose
		// required index keys cannot all be derived.
		if lvl, ok := built["conformance_level"].(string); ok {
			entry.ConformanceLevel = lvl
		}
		if missing := index.MissingRequiredKeys(built); len(missing) > 0 {
			fmt.Fprintf(os.Stderr, "ulc from-sheet: %s: builder cannot derive required index keys:\n", findings.SanitizeText(res.RecordID))
			for _, key := range missing {
				src := index.RequiredKeySources[key]
				if src == "" {
					src = "(always-present marker)"
				}
				fmt.Fprintf(os.Stderr, "  - %s  (populate %s)\n", key, src)
			}
			entry.Status = "failed"
			entry.MissingIndexKeys = missing
			doc.Records = append(doc.Records, entry)
			doc.Summary.Failed++
			failed = true
			continue
		}
		res.Record["index"] = built

		outPath := filepath.Join(outDir, res.RecordID+".ulc.json")

		// A record that references files not present on disk carries placeholder
		// (zero-sentinel) hashes under --allow-missing-files. It is a DRAFT, not a
		// validated record, so it is never written to --out (the run also exits
		// non-zero below). With --draft-out it is saved under a distinct
		// .draft.json suffix so it cannot be mistaken for a validated record.
		// The draft write happens before schema validation, so the record_id has
		// not yet been checked against the schema's slug pattern; a draft
		// filename is never built from an unchecked cell.
		if recordSentinel {
			entry.Status = "draft"
			if draftDir != "" {
				if !recordIDSafe(res.RecordID) {
					fmt.Fprintf(os.Stderr, "ulc from-sheet: %s: record_id is not a safe filename component; no draft written\n", findings.SanitizeText(res.RecordID))
					entry.Status = "failed"
					entry.Error = "unsafe record_id for draft filename"
					doc.Records = append(doc.Records, entry)
					doc.Summary.Failed++
					failed = true
					continue
				}
				draftPath := filepath.Join(draftDir, res.RecordID+".draft.json")
				draftBytes, merr := marshalRecord(res.Record)
				if merr != nil {
					fmt.Fprintf(os.Stderr, "ulc from-sheet: %v\n", merr)
					entry.Status = "failed"
					entry.Error = "marshal draft failed"
					doc.Records = append(doc.Records, entry)
					doc.Summary.Failed++
					failed = true
					continue
				}
				if werr := os.WriteFile(draftPath, draftBytes, 0o644); werr != nil {
					fmt.Fprintf(os.Stderr, "ulc from-sheet: write %s: %v\n", findings.SanitizeText(draftPath), werr)
					entry.Status = "failed"
					entry.Error = "write draft failed"
					doc.Records = append(doc.Records, entry)
					doc.Summary.Failed++
					failed = true
					continue
				}
				entry.DraftPath = draftPath
				if !jsonOut {
					fmt.Printf("%s -> DRAFT, written to %s (references files not present; --allow-missing-files stamped placeholder hashes)\n", findings.SanitizeText(res.RecordID), findings.SanitizeText(draftPath))
				}
			} else if !jsonOut {
				fmt.Printf("%s -> DRAFT, not written (references files not present; --allow-missing-files stamped placeholder hashes)\n", findings.SanitizeText(res.RecordID))
			}
			doc.Records = append(doc.Records, entry)
			doc.Summary.Drafts++
			continue
		}

		// Marshal once and validate the bytes BEFORE writing, so a record that
		// fails schema validation never lands in --out. The validator wants the
		// json.Number-typed tree; decodeStrict reproduces the shape the validate
		// subcommand sees. (sheet.Convert already computed each source file's
		// SHA-256 against the resolved assets root, so no hash re-verification is
		// needed here.)
		recordBytes, merr := marshalRecord(res.Record)
		if merr != nil {
			fmt.Fprintf(os.Stderr, "ulc from-sheet: %v\n", merr)
			entry.Status = "failed"
			entry.Error = "marshal record failed"
			doc.Records = append(doc.Records, entry)
			doc.Summary.Failed++
			failed = true
			continue
		}
		report := findings.NewReport()
		// from-sheet's text mode exposes no --verbose, so drop that advice from
		// the hidden-findings hint if the report is rendered on a failure.
		report.OmitFlagHint = true
		rawTree, derr := decodeStrict(recordBytes)
		if derr != nil {
			fmt.Fprintf(os.Stderr, "ulc from-sheet: parse %s: %v\n", findings.SanitizeText(res.RecordID), derr)
			entry.Status = "failed"
			entry.Error = "reparse assembled record failed"
			doc.Records = append(doc.Records, entry)
			doc.Summary.Failed++
			failed = true
			continue
		}
		v.Validate(rawTree, report)
		completeness.Report(res.Record, report)
		achievements.Report(res.Record, report)
		report.Finalize()

		// conformance_level is always stamped (the grader floors at `incomplete`),
		// so the level string is never empty. entry.ConformanceLevel was set
		// right after index build; this local is for the human-readable lines.
		level, _ := built["conformance_level"].(string)
		entry.Findings = report
		if report.HasErrors() {
			entry.Status = "failed"
			doc.Records = append(doc.Records, entry)
			doc.Summary.Failed++
			if !jsonOut {
				fmt.Printf("%s -> %s (%d findings, not written)\n", findings.SanitizeText(res.RecordID), level, len(report.Findings))
				if err := report.WriteText(os.Stderr, outPath); err != nil {
					fmt.Fprintf(os.Stderr, "ulc from-sheet: write report: %v\n", err)
				}
			}
			failed = true
			continue
		}
		if err := os.WriteFile(outPath, recordBytes, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "ulc from-sheet: write %s: %v\n", findings.SanitizeText(outPath), err)
			entry.Status = "failed"
			entry.Error = "write record failed"
			doc.Records = append(doc.Records, entry)
			doc.Summary.Failed++
			failed = true
			continue
		}
		entry.Status = "written"
		entry.Path = outPath
		doc.Records = append(doc.Records, entry)
		doc.Summary.Written++
		if !jsonOut {
			// `incomplete` is a valid, expected outcome (the floor, below core):
			// write the record and flag that it is not yet a publishable grade.
			// The roadmap in the report names what core still needs.
			if level == completeness.LevelIncomplete.String() {
				fmt.Printf("%s -> incomplete (below core; see roadmap) (%d findings)\n", findings.SanitizeText(res.RecordID), len(report.Findings))
			} else {
				fmt.Printf("%s -> %s (%d findings)\n", findings.SanitizeText(res.RecordID), level, len(report.Findings))
			}
		}
	}

	if jsonOut {
		if rc := printJSON("from-sheet", doc, true); rc != 0 {
			return rc
		}
	}
	if sawSentinel {
		fmt.Fprintln(os.Stderr, "ulc from-sheet: one or more records reference files that were not hashed (--allow-missing-files stamped the zero-sentinel SHA-256). These are DRAFTS, not validated records; re-run without --allow-missing-files once the files are present.")
		return 1
	}
	if failed {
		return 1
	}
	return 0
}

// printJSON encodes v as two-space-indented JSON and writes it to stdout in a
// single call, so an encode failure leaves nothing at all on stdout. (A write
// failure is different: os.Stdout.Write can report an error after a partial
// write, which no buffering can prevent. Both are reported on stderr, so an exit
// code of 1 always comes with a diagnostic.)
//
// escapeHTML selects the encoder's treatment of <, > and &: build-index leaves
// them literal to preserve the record's own byte shape, while scope escapes them
// because its document carries record-controlled strings a consumer may inline
// into a page. Validate's --json report escapes them for the same reason (see
// findings.WriteJSON, which owns that encoder). from-sheet's --json conversion
// report escapes them too, for the same reason: it carries record_id values
// and findings text straight from the workbook. Both directions are pinned by
// tests; the setting is public contract for each subcommand, not an
// implementation detail.
func printJSON(subcommand string, v any, escapeHTML bool) int {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(escapeHTML)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "ulc %s: %v\n", subcommand, err)
		return 1
	}
	if _, err := os.Stdout.Write(buf.Bytes()); err != nil {
		fmt.Fprintf(os.Stderr, "ulc %s: write stdout: %v\n", subcommand, err)
		return 1
	}
	return 0
}

func printIndex(idx index.Index) int {
	return printJSON("build-index", idx, false)
}

// --- scope ---

// scopeContractVersion is the semver of the grading-scope manifest's own output
// contract, independent of both CLIVersion and index.BuilderVersion. It is
// additive-only: a new field, a new kind value or a new array bumps the minor,
// and no existing field, key string or semantic changes for the lifetime of
// scope_version 1.x. That scoping is deliberate: the ULC version line governs
// the schema surface, which this manifest is not part of.
const scopeContractVersion = "1.0.0"

// scopeItemOut is one graded item of the manifest. source_document and standard
// reuse the JSON names the findings contract already ships, and every field here
// is a static rubric string, never record input.
type scopeItemOut struct {
	Tier           string `json:"tier"`
	Kind           string `json:"kind"`
	Path           string `json:"path"`
	SourceDocument string `json:"source_document"`
	Standard       string `json:"standard"`
}

// scopeDoc is the manifest document. record_id and ulc_version are the one place
// it carries record input: each is echoed when the record holds a non-empty
// string there, and omitted when the key is absent, non-string, or empty.
//
// `ulc scope` runs no schema validation, so BOTH fields are unvalidated,
// record-controlled strings even though the surrounding document is
// CLI-authored. They are echoed rather than dropped because a consumer that fans
// out over a corpus and concatenates manifests loses the record pairing
// irrecoverably without record_id, and non-conforming records are exactly the
// ones being triaged. A consumer must therefore treat both as untrusted input:
// never a filesystem path, a cache key, or a trust anchor, and escaped for
// whatever output context it lands in. Every other field is a static rubric or
// CLI string.
type scopeDoc struct {
	ScopeVersion string         `json:"scope_version"`
	CLIVersion   string         `json:"cli_version"`
	RecordID     string         `json:"record_id,omitempty"`
	ULCVersion   string         `json:"ulc_version,omitempty"`
	Blocks       []string       `json:"blocks"`
	Items        []scopeItemOut `json:"items"`
}

// buildScopeDoc projects the rubric's scope for record into the output document.
// It is a pure DTO mapping: the scope decision and the blocks rollup both belong
// to internal/completeness, which owns the rubric and tests them.
func buildScopeDoc(record map[string]any) scopeDoc {
	items := completeness.Scope(record)
	doc := scopeDoc{
		ScopeVersion: scopeContractVersion,
		CLIVersion:   CLIVersion,
		Blocks:       completeness.RollupBlocks(items),
		Items:        make([]scopeItemOut, 0, len(items)),
	}
	if s, ok := record["record_id"].(string); ok {
		doc.RecordID = s
	}
	if s, ok := record["ulc_version"].(string); ok {
		doc.ULCVersion = s
	}
	for _, it := range items {
		doc.Items = append(doc.Items, scopeItemOut{
			Tier:           it.Level.String(),
			Kind:           string(it.Kind),
			Path:           it.Path,
			SourceDocument: it.Document,
			Standard:       it.Standard,
		})
	}
	return doc
}

func runScope(args []string) int {
	// ContinueOnError, deliberately unlike validate, build-index and from-sheet:
	// it keeps -h and an unrecognized flag as return values rather than an
	// os.Exit inside the process, so both stay assertable. Parse has already
	// printed the usage block in each case, so neither arm re-prints it.
	fs := flag.NewFlagSet("scope", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `ulc scope -- print the grading-scope manifest for a ULC record.

States, in result form, which top-level blocks and which graded items the
conformance rubric holds in scope for this record: an exit sign is never
asked for photometric distribution data, a downlight is never asked for the
exit_sign dataset. The output names results only, never the predicate logic
that decided them.

The manifest covers the gating tiers (core, standard, full). Non-gating
enrichment and observation guidance stays in `+"`ulc validate`"+`. This is not the
record's `+"`applicability`"+` block, which declares SKU coverage.

Exit codes:
  0   manifest emitted (or -h)
  1   the record could not be read, parsed, or used as a ULC record,
      or the manifest could not be written
  2   usage error

USAGE
    ulc scope <record.ulc>
`)
	}
	if err := fs.Parse(reorderFlagsFirst(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	recordPath := fs.Arg(0)

	record, err := readRecord(recordPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ulc scope: %v\n", err)
		return 1
	}

	return printJSON("scope", buildScopeDoc(record), true)
}

// valueFlags are the flag names across all subcommands that take a
// space-separated value, so reorderFlagsFirst keeps the value riding with its
// flag when partitioning. Boolean flags are absent from this set.
var valueFlags = map[string]bool{
	"-schema-dir": true, "--schema-dir": true,
	"-out": true, "--out": true,
	"-assets": true, "--assets": true,
	"-draft-out": true, "--draft-out": true,
	"-as-of": true, "--as-of": true,
	"-expiry-window": true, "--expiry-window": true,
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

func marshalRecord(record index.Record) ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(record); err != nil {
		return nil, fmt.Errorf("encode record: %w", err)
	}
	return buf.Bytes(), nil
}

// writeRecord marshals the record and writes it to path (used by build-index's
// in-place write; from-sheet marshals then validates before writing).
func writeRecord(path string, record index.Record) error {
	data, err := marshalRecord(record)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
