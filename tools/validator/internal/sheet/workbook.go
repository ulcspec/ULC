// Package sheet implements `ulc from-sheet`: a deterministic (no-LLM)
// converter that turns a manufacturer-authored spreadsheet bundle into valid
// ULC records.
//
// The converter never targets a conformance level. It ingests every authored
// field, assembles the record's deep blocks, computes dual-unit companions,
// SHA-256 hashes, and default provenance, then hands the assembled record to
// the index builder (which stamps the index and grades the achieved
// conformance_level) and the schema validator. See DESIGN.md in this directory
// for the full sheet list, the column-to-path contract, and the resolved
// implementer decisions.
//
// All four authoring patterns are handled over the CSV-bundle input format.
// Patterns A and C share the fixed-axes pin path (C differs only in per-column
// provenance, supplied through the provenance override columns). Patterns B and D
// add the covered-axes assembler: the applicability block plus the
// derivation-generated photometry.declared_by_cct (CCT multiplier table) and
// photometry.declared_by_length (per-foot linear scaling) tables. The full
// long-sheet set beyond these (cie97, alpha_opic, flicker, package) remains an
// optional future input.
package sheet

// Row is one spreadsheet row as a header->cell map. A blank or whitespace-only
// cell is dropped at read time, so a key present in a Row always carries a
// non-empty trimmed value. Callers therefore use the comma-ok form to
// distinguish "authored" from "absent".
type Row map[string]string

// Workbook is a format-agnostic model of a manufacturer workbook: a map from
// sheet name (for example "records", "source_files") to that sheet's rows in
// file order. Any reader (CSV bundle today, native .xlsx as a fast-follow)
// produces this same model, so the assembler is decoupled from the input
// format.
type Workbook map[string][]Row

// Sheet returns the rows for the named sheet and whether the sheet exists.
// A sheet that exists but has no data rows returns an empty, non-nil slice.
func (w Workbook) Sheet(name string) ([]Row, bool) {
	rows, ok := w[name]
	return rows, ok
}

// RowsFor returns the rows of the named sheet whose record_id column equals id,
// in file order. Sheets keyed by record_id (every related sheet) are joined
// against the master records row through this helper. A missing sheet yields no
// rows.
func (w Workbook) RowsFor(name, id string) []Row {
	out := []Row{}
	for _, r := range w[name] {
		if r["record_id"] == id {
			out = append(out, r)
		}
	}
	return out
}
