package sheet

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// ReadXLSX reads a native .xlsx workbook into the same Workbook model produced
// by ReadCSVBundle. Each worksheet (tab) becomes a sheet named by the tab's
// display name, the first row is the header, cells are trimmed, blank cells are
// dropped, and fully blank rows are skipped, exactly as the CSV reader does.
// Equivalent data authored as an .xlsx or as a CSV bundle therefore produces
// identical Workbooks, so the manufacturer can hand the converter either one.
//
// Only the Go standard library is used (archive/zip + encoding/xml), so the
// reader stays offline and dependency-free, the same constraint the CSV reader
// honors. The tab names must equal the sheet names the converter joins on
// (records, source_files, attestations, ...); the published workbook template
// ships its tabs already named that way.
//
// The reader is faithful to each cell's stored text, not to Excel's display
// formatting: it does not consult xl/styles.xml, so a date authored as an Excel
// date cell reads as its serial number and a number reads at full stored
// precision. Authors should type dates as ISO strings and numbers as plain
// numbers (see templates/workbook/README.md), which read identically from a CSV
// bundle and an .xlsx. Error cells (t="e") read as absent.
func ReadXLSX(filePath string) (Workbook, error) {
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("open xlsx %s: %w", filePath, err)
	}
	defer zr.Close()

	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}

	var wbXML xlWorkbook
	if err := decodeXMLPart(files, "xl/workbook.xml", &wbXML); err != nil {
		return nil, fmt.Errorf("xlsx %s: %w", filePath, err)
	}

	// The relationships map (r:id -> worksheet part path) is how a sheet's
	// display name resolves to its XML part. Most files have it; tolerate its
	// absence only to produce a clearer downstream error.
	relTarget := map[string]string{}
	var rels xlRels
	if err := decodeXMLPart(files, "xl/_rels/workbook.xml.rels", &rels); err == nil {
		for _, r := range rels.Relationships {
			if r.TargetMode == "External" {
				continue
			}
			relTarget[r.ID] = r.Target
		}
	}

	table, err := readSharedStrings(files)
	if err != nil {
		return nil, fmt.Errorf("xlsx %s: %w", filePath, err)
	}

	wb := Workbook{}
	for _, s := range wbXML.Sheets {
		target, ok := relTarget[s.RID]
		if !ok {
			return nil, fmt.Errorf("xlsx %s: sheet %q references relationship %q with no target", filePath, s.Name, s.RID)
		}
		partPath := resolveWorkbookRel(target)
		f, ok := files[partPath]
		if !ok {
			return nil, fmt.Errorf("xlsx %s: sheet %q worksheet part %q not found", filePath, s.Name, partPath)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("xlsx %s: open worksheet %q: %w", filePath, partPath, err)
		}
		rows, err := readWorksheet(rc, table)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("xlsx %s: sheet %q: %w", filePath, s.Name, err)
		}
		wb[strings.TrimSpace(s.Name)] = rows
	}
	return wb, nil
}

// --- OOXML part structs ---

// xlWorkbook is xl/workbook.xml: the ordered list of sheets, each naming a tab
// and its relationship id.
type xlWorkbook struct {
	XMLName xml.Name     `xml:"workbook"`
	Sheets  []xlSheetRef `xml:"sheets>sheet"`
}

// xlSheetRef is one <sheet name= r:id= sheetId=>. The RID tag matches the local
// name "id"; encoding/xml matches on local name, so it captures r:id regardless
// of the namespace prefix the writer used.
type xlSheetRef struct {
	Name string `xml:"name,attr"`
	RID  string `xml:"id,attr"`
}

// xlRels is xl/_rels/workbook.xml.rels: relationship id -> worksheet part path.
type xlRels struct {
	XMLName       xml.Name         `xml:"Relationships"`
	Relationships []xlRelationship `xml:"Relationship"`
}

type xlRelationship struct {
	ID         string `xml:"Id,attr"`
	Target     string `xml:"Target,attr"`
	TargetMode string `xml:"TargetMode,attr"`
}

// xlSST is xl/sharedStrings.xml: the shared string table. Each <si> is one
// entry addressed by 0-based position.
type xlSST struct {
	XMLName xml.Name `xml:"sst"`
	SI      []xlSI   `xml:"si"`
}

// xlSI captures both string shapes: a simple <si><t>..</t></si> (T set) and a
// rich-run <si><r><t>..</t></r>..</si> (R set). The logical value is the
// concatenation of every <t> descendant; exactly one shape is populated in
// practice, so concatenating both is safe.
type xlSI struct {
	T string  `xml:"t"`
	R []xlRun `xml:"r"`
}

type xlRun struct {
	T string `xml:"t"`
}

// xlRow is one <row> with its cells; the r attribute (1-based row number) is
// informational and unused (row position is implied by document order).
type xlRow struct {
	Cells []xlCell `xml:"c"`
}

// xlCell is one <c r= t=> with a <v> element (or an <is> inline string). The r
// attribute is the A1 reference; t selects how the logical value is read.
type xlCell struct {
	R      string    `xml:"r,attr"`
	T      string    `xml:"t,attr"`
	V      string    `xml:"v"`
	Inline *xlInline `xml:"is"`
}

// xlInline is the <is> child of an inlineStr cell, mirroring xlSI's two shapes.
type xlInline struct {
	T string  `xml:"t"`
	R []xlRun `xml:"r"`
}

// text concatenates the simple and rich-run text of a shared-string entry.
func (si xlSI) text() string {
	out := si.T
	for _, r := range si.R {
		out += r.T
	}
	return out
}

// text concatenates the simple and rich-run text of an inline string cell.
func (in xlInline) text() string {
	out := in.T
	for _, r := range in.R {
		out += r.T
	}
	return out
}

// --- readers ---

// decodeXMLPart decodes a required XML part into v, erroring when the part is
// absent (used for xl/workbook.xml; the .rels part is decoded best-effort by the
// caller).
func decodeXMLPart(files map[string]*zip.File, name string, v any) error {
	f, ok := files[name]
	if !ok {
		return fmt.Errorf("missing part %s", name)
	}
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer rc.Close()
	if err := xml.NewDecoder(rc).Decode(v); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}

// readSharedStrings reads the shared string table, returning an empty (non-nil)
// table when xl/sharedStrings.xml is absent (a file with no shared strings).
func readSharedStrings(files map[string]*zip.File) ([]string, error) {
	f, ok := files["xl/sharedStrings.xml"]
	if !ok {
		return []string{}, nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open sharedStrings: %w", err)
	}
	defer rc.Close()
	var sst xlSST
	if err := xml.NewDecoder(rc).Decode(&sst); err != nil {
		return nil, fmt.Errorf("decode sharedStrings: %w", err)
	}
	out := make([]string, len(sst.SI))
	for i, si := range sst.SI {
		out[i] = si.text()
	}
	return out, nil
}

// readWorksheet streams a worksheet's rows, decoding one <row> at a time to keep
// memory flat on large long sheets. The first row is the header; subsequent rows
// are densified from their A1 cell references and run through the same trim /
// drop-blank-cell / skip-blank-row logic as the CSV reader, so an .xlsx and an
// equivalent CSV bundle yield identical Rows.
func readWorksheet(rc io.Reader, table []string) ([]Row, error) {
	dec := xml.NewDecoder(rc)
	var header []string // nil until the first data row is seen
	rows := []Row{}
	inSheetData := false
	var sheetDataNS string
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		switch se := tok.(type) {
		case xml.StartElement:
			// Enter <sheetData>; capture its namespace so only same-namespace
			// <row> children count as spreadsheet rows.
			if se.Name.Local == "sheetData" {
				inSheetData = true
				sheetDataNS = se.Name.Space
				continue
			}
			// A real row is a <row> in the sheetData namespace, inside <sheetData>.
			// Scoping this way rejects a foreign-namespace <row> that the OOXML
			// extension mechanism (extLst) may legally place elsewhere in the
			// worksheet: without the scope such an element would be mis-read as the
			// header and silently drop every real data row.
			if !inSheetData || se.Name.Local != "row" || se.Name.Space != sheetDataNS {
				continue
			}
			var xr xlRow
			if err := dec.DecodeElement(&xr, &se); err != nil {
				return nil, err
			}

			// Densify: column index -> logical (untrimmed) value. Missing cells
			// leave gaps that read as blank, matching the CSV reader's "absent".
			dense := map[int]string{}
			for _, c := range xr.Cells {
				ci := colIndex(c.R)
				if ci < 0 {
					continue
				}
				dense[ci] = cellValue(c, table)
			}

			if header == nil {
				header = buildHeader(dense)
				continue
			}
			row := Row{}
			allBlank := true
			for i, col := range header {
				if col == "" {
					continue
				}
				v := strings.TrimSpace(dense[i])
				if v == "" {
					continue // blank cell = absent
				}
				row[col] = v
				allBlank = false
			}
			if allBlank {
				continue
			}
			rows = append(rows, row)
		case xml.EndElement:
			if se.Name.Local == "sheetData" {
				inSheetData = false
			}
		}
	}
	return rows, nil
}

// buildHeader turns the densified first row into a positional header slice
// indexed by 0-based column. Gaps (missing columns) become "" and are skipped by
// the data loop, mirroring the CSV reader's empty-header-name skip. Header names
// are trimmed.
func buildHeader(dense map[int]string) []string {
	maxCol := -1
	for ci := range dense {
		if ci > maxCol {
			maxCol = ci
		}
	}
	h := make([]string, maxCol+1)
	for ci, v := range dense {
		h[ci] = strings.TrimSpace(v)
	}
	return h
}

// cellValue resolves one cell's logical string by its type. Shared strings
// resolve through the table; booleans render TRUE/FALSE (matching a CSV export
// of the same cell, which parseBool accepts); numbers, formula strings, errors,
// and ISO dates pass their raw <v> through verbatim for the converter to parse.
func cellValue(c xlCell, table []string) string {
	switch c.T {
	case "s":
		idx, err := strconv.Atoi(strings.TrimSpace(c.V))
		if err != nil || idx < 0 || idx >= len(table) {
			return ""
		}
		return table[idx]
	case "inlineStr":
		if c.Inline != nil {
			return c.Inline.text()
		}
		return ""
	case "b":
		switch strings.TrimSpace(c.V) {
		case "1":
			return "TRUE"
		case "0":
			return "FALSE"
		default:
			// A conformant t="b" cell is always "0" or "1"; a malformed value is
			// treated as absent rather than leaked downstream as a stray token.
			return ""
		}
	case "e":
		// Error cell (#DIV/0!, #N/A, #REF!, ...): a broken formula, not data.
		// Treat as absent so it cannot corrupt a numeric field downstream.
		return ""
	default:
		// "" (number), "n", "str" (formula string), "d" (ISO date): raw <v>
		// verbatim for the converter to parse.
		return c.V
	}
}

// colIndex parses the leading letters of an A1 cell reference ("B", "AA", "ZZ")
// into a 0-based column index, ignoring the trailing row digits. Bijective
// base-26: A=1..Z=26, AA=27, so the 0-based result is (accumulated - 1). Returns
// -1 for a reference with no leading letter (malformed); callers skip it.
func colIndex(ref string) int {
	n := 0
	i := 0
	for ; i < len(ref); i++ {
		ch := ref[i]
		switch {
		case ch >= 'A' && ch <= 'Z':
			n = n*26 + int(ch-'A'+1)
		case ch >= 'a' && ch <= 'z':
			n = n*26 + int(ch-'a'+1)
		default:
			i = len(ref) // stop at the first non-letter (the row digits)
		}
	}
	if n == 0 {
		return -1
	}
	return n - 1
}

// resolveWorkbookRel resolves a workbook relationship Target (relative to the
// xl/ directory that holds workbook.xml) to its full ZIP-internal path. OPC
// relationship targets are URI-style, so a space in a part name arrives
// percent-encoded ("sheet%201.xml") while the ZIP entry is the literal name;
// the target is percent-decoded first, and stray backslashes from non-conformant
// writers are normalized to forward slashes. ZIP paths are POSIX forward-slash,
// so the path package (not path/filepath) is used for OS independence. An
// absolute target (leading slash) is taken as-is sans the slash; a relative
// target is joined under xl/ and cleaned of any ../ segments.
func resolveWorkbookRel(target string) string {
	target = strings.ReplaceAll(target, `\`, "/")
	if decoded, err := url.PathUnescape(target); err == nil {
		target = decoded
	}
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(target, "/")
	}
	return path.Clean(path.Join("xl", target))
}
