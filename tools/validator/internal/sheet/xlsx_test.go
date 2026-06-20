package sheet

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/grade"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
	"github.com/ulcspec/ULC/tools/validator/internal/validate"
)

// xcell is one cell in the test xlsx builder: its column letter ("A"), its OOXML
// type ("s" shared string, "n" number, "b" boolean, "i" inlineStr), and its text.
type xcell struct {
	col  string
	typ  string
	text string
}

type xrow []xcell

type xsheet struct {
	name string
	rows []xrow
}

// buildXLSX writes a minimal but genuinely-shaped OOXML .xlsx to outPath: a ZIP
// carrying xl/workbook.xml (with the r: namespace and r:id), the relationships
// part, a workbook-global xl/sharedStrings.xml, and one worksheet per sheet. It
// is a dumb literal-XML emitter (it does not use the reader under test), so the
// parity it proves is real. The emitted parts are exactly the ones ReadXLSX
// consumes, in the shapes Excel produces.
func buildXLSX(t *testing.T, outPath string, sheets []xsheet) {
	t.Helper()

	idx := map[string]int{}
	var sst []string
	intern := func(s string) int {
		if i, ok := idx[s]; ok {
			return i
		}
		i := len(sst)
		sst = append(sst, s)
		idx[s] = i
		return i
	}
	for _, sh := range sheets {
		for _, row := range sh.rows {
			for _, c := range row {
				if c.typ == "s" {
					intern(c.text)
				}
			}
		}
	}

	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("create xlsx: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	put := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}

	var wb strings.Builder
	wb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`)
	for i, sh := range sheets {
		fmt.Fprintf(&wb, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, esc(sh.name), i+1, i+1)
	}
	wb.WriteString(`</sheets></workbook>`)
	put("xl/workbook.xml", wb.String())

	var rl strings.Builder
	rl.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := range sheets {
		fmt.Fprintf(&rl, `<Relationship Id="rId%d" `+
			`Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" `+
			`Target="worksheets/sheet%d.xml"/>`, i+1, i+1)
	}
	rl.WriteString(`</Relationships>`)
	put("xl/_rels/workbook.xml.rels", rl.String())

	var ss strings.Builder
	fmt.Fprintf(&ss, `<?xml version="1.0" encoding="UTF-8"?>`+
		`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">`,
		len(sst), len(sst))
	for _, s := range sst {
		fmt.Fprintf(&ss, `<si><t xml:space="preserve">%s</t></si>`, esc(s))
	}
	ss.WriteString(`</sst>`)
	put("xl/sharedStrings.xml", ss.String())

	for i, sh := range sheets {
		var w strings.Builder
		w.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` +
			`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
		for r, row := range sh.rows {
			fmt.Fprintf(&w, `<row r="%d">`, r+1)
			for _, c := range row {
				ref := fmt.Sprintf("%s%d", c.col, r+1)
				switch c.typ {
				case "s":
					fmt.Fprintf(&w, `<c r="%s" t="s"><v>%d</v></c>`, ref, idx[c.text])
				case "n":
					fmt.Fprintf(&w, `<c r="%s"><v>%s</v></c>`, ref, esc(c.text))
				case "b":
					fmt.Fprintf(&w, `<c r="%s" t="b"><v>%s</v></c>`, ref, esc(c.text))
				case "i":
					fmt.Fprintf(&w, `<c r="%s" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, ref, esc(c.text))
				default:
					t.Fatalf("unknown cell type %q", c.typ)
				}
			}
			w.WriteString(`</row>`)
		}
		w.WriteString(`</sheetData></worksheet>`)
		put(fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), w.String())
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

// esc XML-escapes a string for safe literal embedding.
func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// colLetters maps a 0-based column index to its spreadsheet letters (0->A,
// 25->Z, 26->AA), the inverse of the reader's colIndex.
func colLetters(ci int) string {
	s := ""
	ci++
	for ci > 0 {
		ci--
		s = string(rune('A'+ci%26)) + s
		ci /= 26
	}
	return s
}

// TestReadXLSXMatchesCSVBundle is the parity contract: an .xlsx whose tabs mirror
// the Pattern-A CSV bundle's sheets, cell for cell, must read into a Workbook
// byte-identical to ReadCSVBundle of that bundle. This locks the promise that a
// manufacturer may hand the converter either input shape.
func TestReadXLSXMatchesCSVBundle(t *testing.T) {
	bundle := filepath.Join("testdata", "bundle")

	xlsxPath := filepath.Join(t.TempDir(), "bundle.xlsx")
	buildXLSX(t, xlsxPath, bundleToXLSXSheets(t, bundle))

	got, err := ReadXLSX(xlsxPath)
	if err != nil {
		t.Fatalf("ReadXLSX: %v", err)
	}
	want, err := ReadCSVBundle(bundle)
	if err != nil {
		t.Fatalf("ReadCSVBundle: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		for name, wrows := range want {
			grows := got[name]
			if !reflect.DeepEqual(grows, wrows) {
				t.Errorf("sheet %q differs:\n  xlsx: %v\n  csv:  %v", name, grows, wrows)
			}
		}
		t.Fatalf("ReadXLSX != ReadCSVBundle (sheets: xlsx=%d csv=%d)", len(got), len(want))
	}
}

// TestConvertFromXLSX drives the whole pipeline through the .xlsx path: an .xlsx
// mirroring the Pattern-B fixture (including the comprehensive long sheets), with
// the referenced asset files alongside it, classifies as Pattern B, grades standard
// (the same honest level the CSV bundle reaches under the redesigned rubric), and
// validates against the live schema with zero ERROR findings, just as the CSV
// bundle does. This covers the Convert input dispatch, the assets-root default
// (the .xlsx's parent directory), and SHA-256 resolution.
func TestConvertFromXLSX(t *testing.T) {
	src := filepath.Join("testdata", "bundle-b")
	dir := t.TempDir()
	// Copy the referenced asset files (cutsheet PDF, IES) so hashing resolves.
	copyBundle(t, src, dir)
	xlsxPath := filepath.Join(dir, "workbook.xlsx")
	buildXLSX(t, xlsxPath, bundleToXLSXSheets(t, src))

	results, err := Convert(xlsxPath, Options{})
	if err != nil {
		t.Fatalf("Convert(xlsx): %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	res := results[0]
	if res.Pattern != PatternB {
		t.Fatalf("xlsx pattern = %s, want B", res.Pattern)
	}

	built := index.Build(res.Record)
	if missing := index.MissingRequiredKeys(built); len(missing) > 0 {
		t.Fatalf("xlsx: MissingRequiredKeys not empty: %v", missing)
	}
	if got := grade.AchievedLevel(res.Record); got != grade.LevelStandard {
		t.Fatalf("xlsx grade = %s, want standard", got)
	}
	res.Record["index"] = built
	tree := numberTree(t, res.Record)
	v, err := validate.NewValidator(schemaDir(t))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	report := findings.NewReport()
	v.Validate(tree, report)
	validate.VerifyHashes(dir, res.Record, report)
	report.Finalize()
	if report.HasErrors() {
		buf := &bytes.Buffer{}
		_ = report.WriteText(buf, res.RecordID)
		t.Fatalf("xlsx-sourced record produced ERROR findings:\n%s", buf.String())
	}
}

// bundleToXLSXSheets reads every <sheet>.csv in a bundle directory (sorted, like
// ReadCSVBundle) and turns each into an xsheet of shared-string cells, dropping
// blank cells so the mirrored xlsx matches the CSV reader's density.
func bundleToXLSXSheets(t *testing.T, bundleDir string) []xsheet {
	t.Helper()
	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		t.Fatalf("read bundle dir %s: %v", bundleDir, err)
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".csv") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	sheets := make([]xsheet, 0, len(names))
	for _, n := range names {
		grid := readRawCSV(t, filepath.Join(bundleDir, n))
		sh := xsheet{name: strings.TrimSuffix(n, filepath.Ext(n))}
		for _, rec := range grid {
			row := xrow{}
			for ci, val := range rec {
				if strings.TrimSpace(val) == "" {
					continue // sparse, mirroring the CSV reader's blank-cell drop
				}
				row = append(row, xcell{col: colLetters(ci), typ: "s", text: val})
			}
			sh.rows = append(sh.rows, row)
		}
		sheets = append(sheets, sh)
	}
	return sheets
}

// TestReadXLSXCellTypes exercises every cellValue branch and the sparse-cell /
// blank-row rules: a shared-string header, a column gap (absent cell), a boolean
// rendered TRUE, an inline string (trimmed), a number cell, and a fully blank
// row that is skipped.
func TestReadXLSXCellTypes(t *testing.T) {
	sheets := []xsheet{{
		name: "records",
		rows: []xrow{
			// header (A..D)
			{{"A", "s", "record_id"}, {"B", "s", "input_power_w"}, {"C", "s", "active"}, {"D", "s", "note"}},
			// r1: A shared, B absent (gap), C boolean 1 -> TRUE, D inline string
			{{"A", "s", "r1"}, {"C", "b", "1"}, {"D", "i", "hello"}},
			// r2: A shared, B number, D inline with surrounding spaces (trimmed)
			{{"A", "s", "r2"}, {"B", "n", "42"}, {"D", "i", "  spaced  "}},
			// fully blank row -> skipped
			{},
		},
	}}
	path := filepath.Join(t.TempDir(), "types.xlsx")
	buildXLSX(t, path, sheets)

	got, err := ReadXLSX(path)
	if err != nil {
		t.Fatalf("ReadXLSX: %v", err)
	}
	want := Workbook{"records": []Row{
		{"record_id": "r1", "active": "TRUE", "note": "hello"},
		{"record_id": "r2", "input_power_w": "42", "note": "spaced"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cell-type mismatch:\n got: %v\nwant: %v", got, want)
	}
}

// TestColIndex checks the A1 column-letter parser across single, double, and
// malformed references.
func TestColIndex(t *testing.T) {
	cases := []struct {
		ref  string
		want int
	}{
		{"A1", 0}, {"B7", 1}, {"Z1", 25}, {"AA1", 26}, {"AB10", 27}, {"ZZ99", 701},
		{"a1", 0}, {"", -1}, {"1", -1},
	}
	for _, c := range cases {
		if got := colIndex(c.ref); got != c.want {
			t.Errorf("colIndex(%q) = %d, want %d", c.ref, got, c.want)
		}
	}
}

// writeRawXLSX zips arbitrary part-name -> content pairs into an .xlsx at path,
// so a test can author the exact OOXML (real-Excel preambles, foreign-namespace
// elements, phonetic runs, error cells) that the structured builder above does
// not emit.
func writeRawXLSX(t *testing.T, outPath string, parts map[string]string) {
	t.Helper()
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("create xlsx: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

// TestReadXLSXRealExcelShapes feeds the reader the OOXML shapes a real Excel /
// LibreOffice file carries that the structured builder does not: a worksheet
// preamble (dimension / sheetViews / cols), a FOREIGN-namespace <row> placed
// before <sheetData> (which must NOT become the header), an error cell t="e"
// (which must read as absent), a formula-string cell t="str", and a shared
// string with rich <r> runs plus a phonetic <rPh> run (whose text must be
// excluded). These guard the namespace/sheetData scoping and the error-cell
// handling against regression.
func TestReadXLSXRealExcelShapes(t *testing.T) {
	const ns = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"`
	parts := map[string]string{
		"xl/workbook.xml": `<?xml version="1.0"?><workbook ` + ns +
			` xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets><sheet name="records" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
			`</Relationships>`,
		// index 0 = rich-run "Acme"+"Co" with a phonetic rPh that must be ignored;
		// 1 = "record_id"; 2 = "note"; 3 = "r1".
		"xl/sharedStrings.xml": `<?xml version="1.0"?><sst ` + ns + `>` +
			`<si><r><t>Acme</t></r><r><t>Co</t></r><rPh sb="0" eb="2"><t>IGNORE</t></rPh></si>` +
			`<si><t>record_id</t></si><si><t>note</t></si><si><t>r1</t></si>` +
			`</sst>`,
		"xl/worksheets/sheet1.xml": `<?xml version="1.0"?><worksheet ` + ns +
			` xmlns:x14="http://schemas.microsoft.com/office/spreadsheetml/2009/9/main">` +
			`<dimension ref="A1:D2"/><sheetViews><sheetView workbookViewId="0"/></sheetViews>` +
			`<cols><col min="1" max="4" width="12"/></cols>` +
			// A foreign-namespace <row> BEFORE sheetData: the regression case. With
			// local-name-only matching it would become an empty header and drop the
			// real rows; scoped matching ignores it.
			`<x14:extLst><x14:row><x14:c r="A1"><v>POISON</v></x14:c></x14:row></x14:extLst>` +
			`<sheetData>` +
			`<row r="1"><c r="A1" t="s"><v>1</v></c><c r="B1" t="s"><v>2</v></c>` +
			`<c r="C1" t="inlineStr"><is><t>status</t></is></c><c r="D1" t="inlineStr"><is><t>maker</t></is></c></row>` +
			`<row r="2"><c r="A2" t="s"><v>3</v></c><c r="B2" t="e"><v>#N/A</v></c>` +
			`<c r="C2" t="str"><v>ok</v></c><c r="D2" t="s"><v>0</v></c></row>` +
			`</sheetData>` +
			// A foreign <row> AFTER sheetData too (the conformant extLst position).
			`<extLst><ext xmlns:x14="urn:x"><x14:row>JUNK</x14:row></ext></extLst>` +
			`</worksheet>`,
	}
	path := filepath.Join(t.TempDir(), "real.xlsx")
	writeRawXLSX(t, path, parts)

	got, err := ReadXLSX(path)
	if err != nil {
		t.Fatalf("ReadXLSX: %v", err)
	}
	// Header is [record_id, note, status, maker]. The data row: A=r1; B is an
	// error cell (absent); C="ok"; D=rich-run shared string "AcmeCo" (rPh dropped).
	want := Workbook{"records": []Row{
		{"record_id": "r1", "status": "ok", "maker": "AcmeCo"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("real-Excel shapes mismatch:\n got: %v\nwant: %v", got, want)
	}
}

// readRawCSV reads a CSV file into a row-major grid, using the same reader
// settings as ReadCSVBundle so the mirrored xlsx carries identical pre-trim
// values.
func readRawCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	recs, err := r.ReadAll()
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return recs
}
