package sheet

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Options configures a conversion run.
type Options struct {
	// AssetsRoot is the directory path-input columns (cutsheet_file,
	// source_files.filename, attestations.source_document_file) resolve against.
	// When empty, the converter uses the bundle directory.
	AssetsRoot string
	// AllowMissingFiles tolerates a referenced file that is absent on disk by
	// stamping the zero-sentinel SHA-256 and recording a warning, instead of
	// failing the conversion.
	AllowMissingFiles bool
}

// Result is one assembled record plus the metadata the command needs to report
// on it. Record is the deep-block map (no index block yet): the caller runs
// index.Build to stamp the index and grade the conformance level, keeping
// assembly separate from build+validate for testability.
type Result struct {
	// RecordID is the records-sheet primary key.
	RecordID string
	// Pattern is the detected authoring pattern.
	Pattern Pattern
	// Record is the assembled deep-block record (index omitted).
	Record map[string]any
	// Warnings are non-fatal advisories (for example missing-file sentinels).
	Warnings []string
}

// readWorkbook reads the converter input into a Workbook, dispatching on the
// input shape: a directory is a CSV bundle (a dir of <sheet>.csv files); a file
// with a .xlsx extension is a native workbook (the same Workbook model, so the
// downstream assembler is identical). It returns the Workbook plus the default
// assets root (where referenced files resolve when Options.AssetsRoot is unset):
// the bundle directory itself for a CSV bundle, or the .xlsx file's parent
// directory for an .xlsx input.
func readWorkbook(input string) (Workbook, string, error) {
	info, err := os.Stat(input)
	if err != nil {
		return nil, "", fmt.Errorf("read input %s: %w", input, err)
	}
	if info.IsDir() {
		wb, err := ReadCSVBundle(input)
		return wb, input, err
	}
	if strings.EqualFold(filepath.Ext(input), ".xlsx") {
		wb, err := ReadXLSX(input)
		return wb, filepath.Dir(input), err
	}
	return nil, "", fmt.Errorf("unsupported input %q: expected a CSV bundle directory or an .xlsx file", input)
}

// Convert reads a CSV bundle directory or an .xlsx workbook from input,
// classifies and assembles each records-sheet row into a ULC deep-block record,
// and returns one Result per record. It performs dual-unit expansion, SHA-256
// hashing (with the cutsheet dual-write), default provenance resolution, the
// source_files / attestations / shared_attestations sheet joins, and the
// optional full-level long-sheet blocks. It does NOT build the index or
// validate: the caller does that, so assembly stays unit-testable in isolation.
//
// All four authoring patterns are handled. A and C share the fixed-axes pin
// assembler (C differs only in per-column provenance, which the provenance
// override columns already support). B and D add the covered-axes assembler:
// the applicability block, the CCT multiplier or per-foot derivation, and the
// generated declared_by_cct / declared_by_length tables.
func Convert(input string, opts Options) ([]Result, error) {
	wb, assetsDefault, err := readWorkbook(input)
	if err != nil {
		return nil, err
	}
	records, ok := wb.Sheet("records")
	if !ok {
		return nil, errors.New("input has no records sheet (records.csv or a 'records' worksheet tab)")
	}
	if len(records) == 0 {
		return nil, errors.New("the records sheet has no data rows")
	}

	assetsRoot := opts.AssetsRoot
	if assetsRoot == "" {
		assetsRoot = assetsDefault
	}

	results := make([]Result, 0, len(records))
	seen := map[string]struct{}{}
	for i, master := range records {
		id := master["record_id"]
		if id == "" {
			return nil, fmt.Errorf("records.csv row %d: missing record_id", i+1)
		}
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("records.csv: duplicate record_id %q (must be unique)", id)
		}
		seen[id] = struct{}{}

		hasher := &fileHasher{assetsRoot: assetsRoot, allowMissing: opts.AllowMissingFiles}
		pattern := detectPattern(wb, id, master)

		rec, err := assembleRecord(wb, id, master, pattern, hasher)
		if err != nil {
			return nil, fmt.Errorf("record %q: %w", id, err)
		}
		results = append(results, Result{
			RecordID: id,
			Pattern:  pattern,
			Record:   rec,
			Warnings: hasher.warnings,
		})
	}
	return results, nil
}

// assembleRecord builds one record's deep blocks from the master row and the
// joined related sheets. The order matters: attestations are assembled first so
// the measured -> attestation_ref auto-link can resolve the single LM-79 id, and
// the cutsheet dual-write feeds source_files.
//
// Patterns A and C share the fixed-axes pin path: the master columns carry the
// headline photometry directly, and C's derived provenance arrives through the
// provenance override columns (prov_method, base_attestation_ref). Patterns B and
// D additionally assemble the applicability block and the derivation-generated
// declared_by_cct / declared_by_length tables.
func assembleRecord(wb Workbook, id string, master Row, pattern Pattern, hasher *fileHasher) (map[string]any, error) {
	rec := map[string]any{
		"record_id": id,
	}
	// ulc_version default per DESIGN.md (overridable by the records column).
	// Tracks the current ULC spec version, matching every shipped example and
	// template; a manufacturer who omits the column gets a current-spec record.
	rec["ulc_version"] = "0.6.0"
	// record_status default: active (overridable below).
	rec["record_status"] = "active"

	// Attestations + shared_attestations first, so provenance can auto-link.
	attestations, err := assembleAttestations(wb, id, hasher)
	if err != nil {
		return nil, err
	}
	lm79ID, lm79Count := lm79Anchor(attestations)
	provCtx := provenanceContext{lm79AttestationID: lm79ID, lm79Count: lm79Count}

	// Master-row scalar columns (identity, taxonomy, mechanical, electrical,
	// photometry, colorimetry) via the data-driven column spec.
	if err := applyColumns(rec, master, provCtx); err != nil {
		return nil, err
	}

	// extensions_json: optional per-record vendor-data overflow that lands at
	// extensions.manufacturer_specific.<slug>. Supports the Pattern C (and any)
	// vendor data that has no schema slot.
	if err := applyExtensions(rec, master); err != nil {
		return nil, err
	}

	// Cutsheet dual-write: records.cutsheet_file -> product_family.cutsheet AND a
	// synthesized source_files[] datasheet_pdf entry.
	cutsheetFile := master["cutsheet_file"]
	if cutsheetFile == "" {
		return nil, errors.New("missing required column cutsheet_file")
	}
	cutsheetRef, err := buildFileReference(cutsheetFile, master, "cutsheet_file", hasher)
	if err != nil {
		return nil, err
	}
	if err := setPath(rec, "product_family.cutsheet", cutsheetRef); err != nil {
		return nil, err
	}

	// source_files (joined sheet) plus the synthesized cutsheet entry.
	sourceFiles, err := assembleSourceFiles(wb, id, cutsheetFile, cutsheetRef, hasher)
	if err != nil {
		return nil, err
	}
	rec["source_files"] = sourceFiles

	if len(attestations) > 0 {
		rec["attestations"] = attestations
	}
	if shared := assembleSharedAttestations(wb, id); len(shared) > 0 {
		if err := setPath(rec, "product_family.shared_attestations", shared); err != nil {
			return nil, err
		}
	}

	// Patterns B and D: the applicability block and the derivation-generated
	// photometry tables. A and C are fixed-axes pins and need neither.
	if pattern == PatternB || pattern == PatternD {
		if err := assembleCoveredAxisRecord(wb, id, master, rec, lm79ID, hasher); err != nil {
			return nil, err
		}
	}

	// Optional comprehensive full-level blocks (alpha-opic, flicker, lumen
	// maintenance package, zonal lumens, LCS zonal lumens, ingredient list, the
	// CIE-97 LMF table), authored on dedicated long sheets and shared by every
	// pattern. provCtx supplies the LM-79 anchor for the measured zonal lumens.
	if err := assembleFullLevelBlocks(wb, id, rec, provCtx); err != nil {
		return nil, err
	}

	return rec, nil
}

// assembleCoveredAxisRecord adds the multi-value-applicability deep blocks shared
// by patterns B and D: the applicability block (applicable_catalog_pattern,
// fixed_axes, covered_axes, excluded_combinations), and the derivation-generated
// photometry table. Pattern B generates photometry.declared_by_cct from the CCT
// multiplier table applied to the measured baseline flux; Pattern D generates (or
// echoes) photometry.declared_by_length from the per-foot rates.
func assembleCoveredAxisRecord(wb Workbook, id string, master Row, rec map[string]any, lm79ID string, hasher *fileHasher) error {
	app, err := assembleApplicability(wb, id, master)
	if err != nil {
		return err
	}
	if len(app) > 0 {
		rec["applicability"] = app
	}

	multipliers, multOrder, err := readMultiplierTable(wb, id)
	if err != nil {
		return err
	}

	// Pattern B: generate photometry.declared_by_cct from the multiplier table.
	if len(multipliers) > 0 {
		baseline, ok := baselineFlux(rec)
		if !ok {
			return fmt.Errorf("record %q: a cct_multipliers table is present but photometry.total_luminous_flux_lm.value (the multiplier baseline) is missing", id)
		}
		baselineCCT := coveredAxisBaseline(app, "cct")
		base := lm79ID
		if ref := master["total_luminous_flux_lm__attestation_ref"]; ref != "" {
			base = ref
		}
		if base == "" {
			return fmt.Errorf("record %q: a cct_multipliers table generates scaled declared_by_cct rows, but the record declares no single lm_79* attestation to anchor the measured baseline and the scaled rows' provenance.base_attestation_ref (add a single lm_79* attestations row, or set total_luminous_flux_lm__attestation_ref to disambiguate)", id)
		}
		declared := generateDeclaredByCCT(multipliers, multOrder, baseline, baselineCCT, base)
		if err := setPath(rec, "photometry.declared_by_cct", declared); err != nil {
			return err
		}
	}

	// Pattern D: generate or echo photometry.declared_by_length from the per-foot
	// rates and the covered length axis values.
	if isLengthScaled(wb, id, app) {
		params := declaredByLengthParams{
			rates:        ratesFromRecord(rec),
			lengthValues: coveredAxisValues(app, "length"),
			baseLM79:     lengthBaseAttestation(master, lm79ID),
			baselineIn:   coveredAxisBaseline(app, "length"),
		}
		declared, err := assembleDeclaredByLength(wb, id, params, &hasher.warnings)
		if err != nil {
			return err
		}
		if len(declared) > 0 {
			if err := setPath(rec, "photometry.declared_by_length", declared); err != nil {
				return err
			}
		}
	}

	return nil
}

// applyExtensions writes the optional extensions_json cell into
// extensions.manufacturer_specific.<slug>, where <slug> is the
// extensions_slug column (falling back to manufacturer_slug). The cell is a JSON
// object of vendor data that has no schema slot (the Pattern C vendor-data
// overflow, though any pattern may use it). A blank cell is a no-op.
func applyExtensions(rec map[string]any, master Row) error {
	raw, ok := master["extensions_json"]
	if !ok {
		return nil
	}
	obj, err := parseJSONObjectCell("extensions_json", raw)
	if err != nil {
		return err
	}
	slug := master["extensions_slug"]
	if slug == "" {
		slug = master["manufacturer_slug"]
	}
	if slug == "" {
		return errors.New("extensions_json is set but neither extensions_slug nor manufacturer_slug is present to key extensions.manufacturer_specific")
	}
	return setPath(rec, "extensions.manufacturer_specific."+slug, obj)
}

// applyColumns dispatches each records-sheet column onto the record per its Kind,
// skipping columns the manufacturer left blank.
func applyColumns(rec map[string]any, row Row, provCtx provenanceContext) error {
	for _, col := range recordColumns {
		raw, ok := row[col.Header]
		if !ok {
			continue // blank cell = absent
		}
		val, err := coerceColumn(col, raw, row, provCtx)
		if err != nil {
			return fmt.Errorf("column %q: %w", col.Header, err)
		}
		if err := setPath(rec, col.Path, val); err != nil {
			return err
		}
	}
	return nil
}

// coerceColumn turns one authored cell into the JSON value its Kind dictates.
func coerceColumn(col Column, raw string, row Row, provCtx provenanceContext) (any, error) {
	switch col.Kind {
	case KindString, KindEnum, KindDate:
		return raw, nil
	case KindNumber:
		return parseNumber(raw)
	case KindBool:
		return parseBool(raw)
	case KindList:
		return parseList(raw), nil
	case KindProvNumber:
		num, err := parseNumber(raw)
		if err != nil {
			return nil, err
		}
		rp, err := resolveProvenance(col, row, provCtx)
		if err != nil {
			return nil, err
		}
		obj := map[string]any{"value": num, "value_type": rp.valueType}
		if col.Unit != "" {
			obj["unit"] = col.Unit
		}
		if len(rp.provenance) > 0 {
			obj["provenance"] = rp.provenance
		}
		return obj, nil
	case KindDualUnitSI:
		si, err := parseFloat(raw)
		if err != nil {
			return nil, err
		}
		rp, err := resolveProvenance(col, row, provCtx)
		if err != nil {
			return nil, err
		}
		return buildDualUnit(col.DualKind, si, rp.valueType, rp.provenance), nil
	default:
		return nil, fmt.Errorf("unhandled column kind %d", col.Kind)
	}
}

// --- cell coercion helpers (total, never panic on bad input) ---

// parseNumber returns an int64 for an integral literal, float64 otherwise, so
// the emitted JSON renders whole numbers without a trailing .0.
func parseNumber(raw string) (any, error) {
	if !strings.ContainsAny(raw, ".eE") {
		if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return i, nil
		}
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid number %q", raw)
	}
	return f, nil
}

// parseFloat always returns a float64, used for dual-unit SI leaves where the
// downstream rounding logic operates on floats.
func parseFloat(raw string) (float64, error) {
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", raw)
	}
	return f, nil
}

// parseBool accepts the common spreadsheet truth spellings.
func parseBool(raw string) (bool, error) {
	switch strings.ToLower(raw) {
	case "true", "yes", "y", "1":
		return true, nil
	case "false", "no", "n", "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", raw)
	}
}

// parseList splits a ";"-joined cell into a sorted-stable array of trimmed
// non-empty strings (file order preserved; duplicates kept, matching the
// authored list verbatim).
func parseList(raw string) []any {
	parts := strings.Split(raw, ";")
	out := []any{}
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// --- file-reference + array-sheet assembly ---

// buildFileReference builds a FileReference {filename, sha256, revision_label?,
// revision_date?} from a path-input cell, hashing the file. The override
// columns (`<header>__revision_label`, `<header>__revision_date`) supply the
// optional revision metadata when present.
func buildFileReference(filename string, row Row, header string, hasher *fileHasher) (map[string]any, error) {
	sum, err := hasher.hashFile(filename)
	if err != nil {
		return nil, err
	}
	ref := map[string]any{
		"filename": filename,
		"sha256":   sum,
	}
	if v, ok := row[header+"__revision_label"]; ok {
		ref["revision_label"] = v
	}
	if v, ok := row[header+"__revision_date"]; ok {
		ref["revision_date"] = v
	}
	return ref, nil
}

// assembleSourceFiles builds the source_files[] array from the source_files
// sheet plus the synthesized cutsheet datasheet_pdf entry. The cutsheet entry is
// de-duplicated on filename: if the manufacturer also listed the cutsheet in
// source_files, only one entry survives (the manufacturer-listed one, which may
// carry its own file_type override).
func assembleSourceFiles(wb Workbook, id, cutsheetFilename string, cutsheetRef map[string]any, hasher *fileHasher) ([]any, error) {
	out := []any{}
	seenFilenames := map[string]bool{}

	for _, row := range wb.RowsFor("source_files", id) {
		filename := row["filename"]
		if filename == "" {
			return nil, fmt.Errorf("source_files row for %q: missing filename", id)
		}
		fileType := row["file_type"]
		if fileType == "" {
			return nil, fmt.Errorf("source_files row %q: missing file_type", filename)
		}
		ref, err := buildFileReference(filename, row, "filename", hasher)
		if err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"file_type": fileType,
			"reference": ref,
		})
		seenFilenames[filename] = true
	}

	// Cutsheet dual-write: synthesize a datasheet_pdf entry unless the
	// manufacturer already listed this filename in source_files.
	if !seenFilenames[cutsheetFilename] {
		out = append(out, map[string]any{
			"file_type": "datasheet_pdf",
			"reference": cutsheetRef,
		})
	}
	return out, nil
}

// assembleAttestations builds the top-level attestations[] array from the
// attestations sheet, hashing any source_document_file path-input column into
// source_document_ref.
func assembleAttestations(wb Workbook, id string, hasher *fileHasher) ([]any, error) {
	out := []any{}
	for _, row := range wb.RowsFor("attestations", id) {
		att, err := buildAttestation(row, hasher)
		if err != nil {
			return nil, err
		}
		out = append(out, att)
	}
	return out, nil
}

// assembleSharedAttestations builds product_family.shared_attestations[] from
// the shared_attestations sheet. These are family-wide listings with no
// source-document hashing.
func assembleSharedAttestations(wb Workbook, id string) []any {
	out := []any{}
	for _, row := range wb.RowsFor("shared_attestations", id) {
		out = append(out, buildSharedAttestation(row))
	}
	return out
}

// buildAttestation maps an attestations-sheet row onto an Attestation object.
// The schema requires program and value_type; the rest are optional.
func buildAttestation(row Row, hasher *fileHasher) (map[string]any, error) {
	att := map[string]any{}
	copyIf(att, row, "attestation_id", "attestation_id")
	copyIf(att, row, "program", "program")
	copyIf(att, row, "status", "status")
	copyIf(att, row, "value_type", "value_type")
	copyIf(att, row, "issued_date", "issued_date")
	copyIf(att, row, "test_report_id", "test_report_id")
	copyIf(att, row, "standard_revision", "standard_revision")

	if mq := row["measured_quantities"]; mq != "" {
		att["measured_quantities"] = parseList(mq)
	}
	// verification.type: the schema requires `type` when verification is present;
	// default to unconditional so a bare program row still validates.
	vtype := row["verification_type"]
	if vtype == "" {
		vtype = "unconditional"
	}
	att["verification"] = map[string]any{"type": vtype}

	if doc := row["source_document_file"]; doc != "" {
		ref, err := buildFileReference(doc, row, "source_document_file", hasher)
		if err != nil {
			return nil, err
		}
		att["source_document_ref"] = ref
	}
	if att["program"] == nil {
		return nil, errors.New("attestations row missing required program")
	}
	if att["value_type"] == nil {
		return nil, errors.New("attestations row missing required value_type")
	}
	return att, nil
}

// buildSharedAttestation maps a shared_attestations-sheet row onto an Attestation
// object. Family-wide listings carry a verification block (defaulting to
// unconditional) and no source document.
func buildSharedAttestation(row Row) map[string]any {
	att := map[string]any{}
	copyIf(att, row, "attestation_id", "attestation_id")
	copyIf(att, row, "program", "program")
	copyIf(att, row, "status", "status")
	copyIf(att, row, "value_type", "value_type")
	copyIf(att, row, "standard_revision", "standard_revision")
	vtype := row["verification_type"]
	if vtype == "" {
		vtype = "unconditional"
	}
	att["verification"] = map[string]any{"type": vtype}
	return att
}

// copyIf copies row[src] to dst[key] when the cell is present.
func copyIf(dst map[string]any, row Row, src, key string) {
	if v, ok := row[src]; ok {
		dst[key] = v
	}
}

// lm79Anchor returns the single LM-79-family attestation id used for the
// measured -> attestation_ref auto-link, and the count of LM-79 rows found. The
// id is meaningful only when the count is exactly 1; the provenance resolver
// hard-errors on 0 or >1 when an auto-link is actually needed.
func lm79Anchor(attestations []any) (id string, count int) {
	ids := []string{}
	for _, a := range attestations {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		prog, _ := m["program"].(string)
		if !strings.HasPrefix(prog, "lm_79") {
			continue
		}
		count++
		if aid, ok := m["attestation_id"].(string); ok && aid != "" {
			ids = append(ids, aid)
		}
	}
	sort.Strings(ids)
	if count == 1 && len(ids) == 1 {
		return ids[0], count
	}
	return "", count
}
