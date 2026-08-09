package sheet

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Every hostile payload below lives in a part ReadXLSX actually opens
// (xl/workbook.xml, the workbook rels, xl/sharedStrings.xml, or a referenced
// worksheet). The budget is scoped to opened parts, so a bomb planted in an
// entry the reader never opens is correctly ignored: it costs no memory, and
// charging it would reject legitimate workbooks over their styles or media.

const xlNS = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"`
const relNS = `xmlns="http://schemas.openxmlformats.org/package/2006/relationships"`
const rNS = `xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`

type rawPart struct {
	name string
	data []byte
}

// writeZipParts writes parts to an .xlsx in the given order, so a test can
// control both the content and the entry order of the archive.
func writeZipParts(t *testing.T, outPath string, parts []rawPart) {
	t.Helper()
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("create xlsx: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, p := range parts {
		w, err := zw.Create(p.name)
		if err != nil {
			t.Fatalf("zip create %s: %v", p.name, err)
		}
		if _, err := w.Write(p.data); err != nil {
			t.Fatalf("zip write %s: %v", p.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

// workbookParts returns a well-formed skeleton naming one worksheet per sheet
// name, so a test only has to supply the hostile part.
func workbookParts(sheetNames ...string) []rawPart {
	var sheets, rels strings.Builder
	for i, name := range sheetNames {
		fmt.Fprintf(&sheets, `<sheet name=%q sheetId="%d" r:id="rId%d"/>`, name, i+1, i+1)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Target="worksheets/sheet%d.xml"/>`, i+1, i+1)
	}
	return []rawPart{
		{"xl/workbook.xml", []byte(`<?xml version="1.0"?><workbook ` + xlNS + ` ` + rNS + `><sheets>` + sheets.String() + `</sheets></workbook>`)},
		{"xl/_rels/workbook.xml.rels", []byte(`<?xml version="1.0"?><Relationships ` + relNS + `>` + rels.String() + `</Relationships>`)},
		{"xl/sharedStrings.xml", []byte(`<?xml version="1.0"?><sst ` + xlNS + `></sst>`)},
	}
}

// worksheetWithPadding builds a valid worksheet carrying one header row plus a
// comment of padBytes, which the reader streams past. The padding is what the
// part claims and what the budget is charged.
func worksheetWithPadding(padBytes int, pad func(int) []byte) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0"?><worksheet ` + xlNS + `><!--`)
	b.Write(pad(padBytes))
	b.WriteString(`--><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>header</t></is></c></row></sheetData></worksheet>`)
	return b.Bytes()
}

// zeroPad is highly compressible: a part filled with it deflates at roughly
// 1000:1, so it is only usable where a gate other than the ratio gate is meant
// to trip first.
func zeroPad(n int) []byte { return make([]byte, n) }

// textPad is deliberately only mildly repetitive, so a part filled with it
// stores well under the 100:1 ratio limit. The total-bytes cases need that:
// zero-filled parts would trip the ratio gate before the budget ran out.
func textPad(n int) []byte {
	var b bytes.Buffer
	b.Grow(n + 128)
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "row %09d padding text that is deliberately only mildly repetitive\n", i)
	}
	return b.Bytes()[:n]
}

func wantArchiveLimit(t *testing.T, wb Workbook, err error, limit string) *ArchiveLimitError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an archive limit error, got a workbook with %d sheets", len(wb))
	}
	if wb != nil {
		t.Errorf("expected no workbook alongside the error, got %d sheets", len(wb))
	}
	var lim *ArchiveLimitError
	if !errors.As(err, &lim) {
		t.Fatalf("error is not an *ArchiveLimitError: %v", err)
	}
	if lim.Limit != limit {
		t.Fatalf("Limit = %q, want %q (message: %v)", lim.Limit, limit, err)
	}
	if lim.Observed <= 0 {
		t.Errorf("Observed = %d, want the quantity that tripped the limit", lim.Observed)
	}
	if lim.Max != int64(limitMax(limit)) {
		t.Errorf("Max = %d, want %d", lim.Max, limitMax(limit))
	}
	return lim
}

func limitMax(limit string) int {
	switch limit {
	case limitEntryCount:
		return maxArchiveEntries
	case limitPartBytes:
		return maxPartInflatedBytes
	case limitTotalBytes:
		return maxArchiveInflatedBytes
	case limitCompressionRatio:
		return maxCompressionRatio
	}
	return 0
}

// (a) entry-count bomb.
func TestReadXLSXRejectsTooManyEntries(t *testing.T) {
	out := filepath.Join(t.TempDir(), "entries.xlsx")
	parts := workbookParts("records")
	parts = append(parts, rawPart{"xl/worksheets/sheet1.xml", worksheetWithPadding(0, zeroPad)})
	for i := 0; i < maxArchiveEntries+1; i++ {
		parts = append(parts, rawPart{fmt.Sprintf("docProps/filler%04d.xml", i), []byte("<x/>")})
	}
	writeZipParts(t, out, parts)

	wb, err := ReadXLSX(out)
	lim := wantArchiveLimit(t, wb, err, limitEntryCount)
	if lim.Part != "" {
		t.Errorf("Part = %q, want empty: no single part is at fault for the entry count", lim.Part)
	}
	if !strings.Contains(err.Error(), "the limit is 1024") {
		t.Errorf("message does not name the limit: %v", err)
	}
}

// (b) per-part claim bomb. A 17 MiB claim trips the per-part gate before the
// ratio gate can see it, so zero padding is fine here.
func TestReadXLSXRejectsOversizedPart(t *testing.T) {
	out := filepath.Join(t.TempDir(), "part.xlsx")
	parts := workbookParts("records")
	parts = append(parts, rawPart{"xl/worksheets/sheet1.xml", worksheetWithPadding(17<<20, zeroPad)})
	writeZipParts(t, out, parts)

	wb, err := ReadXLSX(out)
	lim := wantArchiveLimit(t, wb, err, limitPartBytes)
	if lim.Part != "xl/worksheets/sheet1.xml" {
		t.Errorf("Part = %q, want the worksheet", lim.Part)
	}
	if !strings.Contains(err.Error(), "claims") {
		t.Errorf("a gate message must speak of the claim: %v", err)
	}
	// Pin the constant by literal: bracketing it between fixture sizes would
	// let the documented 16 MiB contract drift without a test noticing.
	if !strings.Contains(err.Error(), "the per-part limit is 16777216") {
		t.Errorf("message does not name the 16 MiB per-part limit: %v", err)
	}
}

// (c) total bomb: three opened worksheets, each honest and each under both the
// per-part cap and the ratio limit, summing past the archive total.
func TestReadXLSXRejectsTotalInflation(t *testing.T) {
	out := filepath.Join(t.TempDir(), "total.xlsx")
	const per = 12 << 20
	parts := workbookParts("records", "source_files", "attestations")
	for i := 1; i <= 3; i++ {
		parts = append(parts, rawPart{
			fmt.Sprintf("xl/worksheets/sheet%d.xml", i),
			worksheetWithPadding(per, textPad),
		})
	}
	writeZipParts(t, out, parts)

	// The filler must store under 100:1 or the ratio gate would trip first and
	// this test would pass for the wrong reason.
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "xl/worksheets/") {
			continue
		}
		if f.CompressedSize64 == 0 {
			t.Fatalf("%s stored nothing", f.Name)
		}
		if ratio := f.UncompressedSize64 / f.CompressedSize64; ratio > maxCompressionRatio {
			t.Fatalf("%s stores at %d:1, which would trip the ratio gate instead of the total budget", f.Name, ratio)
		}
	}
	zr.Close()

	wb, err := ReadXLSX(out)
	lim := wantArchiveLimit(t, wb, err, limitTotalBytes)
	if lim.Part == "" {
		t.Errorf("the total-limit error must name the part being read when the budget ran out")
	}
	if !strings.Contains(err.Error(), "total limit") && !strings.Contains(err.Error(), "total inflation limit") {
		t.Errorf("message does not name the total limit: %v", err)
	}
	if !strings.Contains(err.Error(), "33554432") {
		t.Errorf("message does not name the 32 MiB total limit: %v", err)
	}
}

// (d) ratio bomb: above the 1 MiB floor and under the 16 MiB per-part cap, so
// only the ratio limit can trip.
func TestReadXLSXRejectsCompressionRatio(t *testing.T) {
	out := filepath.Join(t.TempDir(), "ratio.xlsx")
	parts := workbookParts("records")
	parts = append(parts, rawPart{"xl/worksheets/sheet1.xml", worksheetWithPadding(2<<20, zeroPad)})
	writeZipParts(t, out, parts)

	wb, err := ReadXLSX(out)
	lim := wantArchiveLimit(t, wb, err, limitCompressionRatio)
	if lim.Observed <= maxCompressionRatio {
		t.Errorf("Observed ratio = %d:1, want above the %d:1 limit", lim.Observed, maxCompressionRatio)
	}
	if !strings.Contains(err.Error(), "the limit is 100:1") {
		t.Errorf("message does not name the ratio limit: %v", err)
	}
}

// The workbook rels part is the fourth thing the reader opens, and its errors
// are otherwise tolerated (an absent or undecodable rels part produces a
// clearer downstream error). A cap violation there must still propagate, or
// the part is capped in name only.
func TestReadXLSXRejectsOversizedRelsPart(t *testing.T) {
	out := filepath.Join(t.TempDir(), "rels.xlsx")
	parts := workbookParts("records")
	for i := range parts {
		if parts[i].name == "xl/_rels/workbook.xml.rels" {
			var b bytes.Buffer
			b.WriteString(`<?xml version="1.0"?><Relationships ` + relNS + `><!--`)
			b.Write(zeroPad(17 << 20))
			b.WriteString(`--></Relationships>`)
			parts[i].data = b.Bytes()
		}
	}
	parts = append(parts, rawPart{"xl/worksheets/sheet1.xml", worksheetWithPadding(0, zeroPad)})
	writeZipParts(t, out, parts)

	wb, err := ReadXLSX(out)
	lim := wantArchiveLimit(t, wb, err, limitPartBytes)
	if lim.Part != "xl/_rels/workbook.xml.rels" {
		t.Errorf("Part = %q, want the rels part", lim.Part)
	}
}

// (e) under-claiming canary. Go's zip reader bounds delivered bytes by the
// central directory's uncompressed size and returns a format error the instant
// a part delivers more than it claimed (archive/zip reader.go:295-304), which
// is what makes the claim gate real enforcement rather than a hint. If a
// future Go relaxes that, this test flips: the counting reader is already in
// place to carry enforcement, and the claim gates would need revisiting.
func TestReadXLSXUnderClaimingPartFailsAsFormatError(t *testing.T) {
	out := filepath.Join(t.TempDir(), "underclaim.xlsx")
	parts := workbookParts("records")
	parts = append(parts, rawPart{"xl/worksheets/sheet1.xml", worksheetWithPadding(0, zeroPad)})
	writeZipParts(t, out, parts)

	if _, err := ReadXLSX(out); err != nil {
		t.Fatalf("fixture must read cleanly before patching: %v", err)
	}
	patchCentralDirectoryUncompressedSize(t, out, "xl/workbook.xml", 10)

	wb, err := ReadXLSX(out)
	if err == nil {
		t.Fatalf("expected an error from the under-claiming part, got %d sheets", len(wb))
	}
	if wb != nil {
		t.Errorf("expected no workbook alongside the error, got %d sheets", len(wb))
	}
	var lim *ArchiveLimitError
	if errors.As(err, &lim) {
		t.Fatalf("under-claiming must surface as a stdlib format error, not an ArchiveLimitError: %v", err)
	}
}

// patchCentralDirectoryUncompressedSize rewrites one entry's uncompressed-size
// field in the central directory, so the archive claims less than the part
// actually delivers. Central directory header: signature 0x02014b50, then a
// 46-byte fixed area with the uncompressed size at offset 24 and the name,
// extra, and comment lengths at 28, 30, and 32.
func patchCentralDirectoryUncompressedSize(t *testing.T, path, part string, size uint32) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	const sig = 0x02014b50
	patched := false
	for i := 0; i+46 <= len(data); i++ {
		if binary.LittleEndian.Uint32(data[i:]) != sig {
			continue
		}
		nameLen := int(binary.LittleEndian.Uint16(data[i+28:]))
		if i+46+nameLen > len(data) {
			continue
		}
		if string(data[i+46:i+46+nameLen]) != part {
			continue
		}
		binary.LittleEndian.PutUint32(data[i+24:], size)
		patched = true
		break
	}
	if !patched {
		t.Fatalf("no central directory entry for %q", part)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
}

// (f) the counting reader on its own, with no archive involved: it must fail
// mid-stream at either limit, and the typed error must survive both plain
// wrapping and encoding/xml.
func TestCountingPartChargesBothLimits(t *testing.T) {
	newPart := func(partAllowance, totalAllowance int64, body string) *countingPart {
		return &countingPart{
			rc:            io.NopCloser(strings.NewReader(body)),
			path:          "fixture.xlsx",
			part:          "xl/worksheets/sheet1.xml",
			partRemaining: partAllowance,
			budget:        &archiveBudget{path: "fixture.xlsx", remaining: totalAllowance},
		}
	}
	body := strings.Repeat("x", 4096)

	t.Run("per_part_limit", func(t *testing.T) {
		_, err := io.Copy(io.Discard, newPart(16, 1<<20, body))
		var lim *ArchiveLimitError
		if !errors.As(err, &lim) {
			t.Fatalf("expected an *ArchiveLimitError, got %v", err)
		}
		if lim.Limit != limitPartBytes {
			t.Errorf("Limit = %q, want %q", lim.Limit, limitPartBytes)
		}
		if !strings.Contains(lim.Error(), "inflated past") {
			t.Errorf("a debit message must not speak of claims: %q", lim.Error())
		}
		if lim.Observed <= 0 {
			t.Errorf("Observed = %d, want the bytes read when the allowance ran out", lim.Observed)
		}
	})

	t.Run("total_limit", func(t *testing.T) {
		_, err := io.Copy(io.Discard, newPart(1<<20, 16, body))
		var lim *ArchiveLimitError
		if !errors.As(err, &lim) {
			t.Fatalf("expected an *ArchiveLimitError, got %v", err)
		}
		if lim.Limit != limitTotalBytes {
			t.Errorf("Limit = %q, want %q", lim.Limit, limitTotalBytes)
		}
		if !strings.Contains(lim.Error(), "took the archive past") {
			t.Errorf("a debit message must not speak of claims: %q", lim.Error())
		}
	})

	t.Run("survives_wrapping", func(t *testing.T) {
		_, err := io.Copy(io.Discard, newPart(16, 1<<20, body))
		wrapped := fmt.Errorf("xlsx %s: %w", "fixture.xlsx", fmt.Errorf("decode %s: %w", "part", err))
		var lim *ArchiveLimitError
		if !errors.As(wrapped, &lim) {
			t.Fatalf("errors.As did not recover the typed error through the wrap chain: %v", wrapped)
		}
	})

	t.Run("survives_encoding_xml", func(t *testing.T) {
		part := newPart(16, 1<<20, `<?xml version="1.0"?><root>`+body+`</root>`)
		var v struct{}
		err := xml.NewDecoder(part).Decode(&v)
		var lim *ArchiveLimitError
		if !errors.As(err, &lim) {
			t.Fatalf("errors.As did not recover the typed error through encoding/xml: %v", err)
		}
	})
}

// A legal workbook must stay legal: the shipped template and the test bundles
// are orders of magnitude under every limit, and no cap may fire on them.
func TestReadXLSXAcceptsLegalWorkbook(t *testing.T) {
	out := filepath.Join(t.TempDir(), "legal.xlsx")
	buildXLSX(t, out, []xsheet{{
		name: "records",
		rows: []xrow{
			{{col: "A", typ: "s", text: "record_id"}, {col: "B", typ: "s", text: "catalog_number"}},
			{{col: "A", typ: "s", text: "acme-orbit-1200"}, {col: "B", typ: "s", text: "ORB-1200"}},
		},
	}})
	wb, err := ReadXLSX(out)
	if err != nil {
		t.Fatalf("a legal workbook must not trip a cap: %v", err)
	}
	if len(wb["records"]) != 1 {
		t.Fatalf("records = %d data rows, want 1 (the first row is the header)", len(wb["records"]))
	}
	if got := wb["records"][0]["catalog_number"]; got != "ORB-1200" {
		t.Errorf("catalog_number = %q, want %q", got, "ORB-1200")
	}
}

// realExcelFixture is an Excel-authored .xlsx of the data-bearing CSV bundle
// beside it: one tab per sheet, tabs named per the workbook convention. It
// lives inside testdata/bundle/ so an .xlsx conversion resolves its assets
// from the same directory the CSV bundle uses (ReadCSVBundle globs *.csv, so
// the workbook is inert there).
const realExcelFixture = "bundle/acme-orbit-1200.xlsx"

// TestReadXLSXRealExcelFixture reads a workbook that desktop Excel actually
// wrote, which no synthetic builder can stand in for. The equivalence
// assertion is the load-bearing one: the reader is faithful to stored text,
// not to display formatting, so a date column left in Excel's default format
// stores as a serial number and a workbook that converts with zero errors can
// still carry values the CSV bundle never had. Only comparing the two readers
// catches that.
func TestReadXLSXRealExcelFixture(t *testing.T) {
	fixture := filepath.Join("testdata", filepath.FromSlash(realExcelFixture))
	if _, err := os.Stat(fixture); errors.Is(err, os.ErrNotExist) {
		t.Skipf("the Excel-authored fixture %s is not committed yet; author it from the CSV bundle beside it, with the date columns and catalog_number formatted as Text before entry", realExcelFixture)
	}

	bundle := filepath.Join("testdata", "bundle")
	fromCSV, err := ReadCSVBundle(bundle)
	if err != nil {
		t.Fatalf("ReadCSVBundle: %v", err)
	}
	fromXLSX, err := ReadXLSX(fixture)
	if err != nil {
		t.Fatalf("ReadXLSX: %v", err)
	}

	// (a) sheet for sheet, cell for cell.
	if len(fromXLSX) != len(fromCSV) {
		t.Fatalf("workbook has %d sheets, the CSV bundle has %d", len(fromXLSX), len(fromCSV))
	}
	for name, csvRows := range fromCSV {
		xlsxRows, ok := fromXLSX[name]
		if !ok {
			t.Errorf("workbook has no %q tab", name)
			continue
		}
		if len(xlsxRows) != len(csvRows) {
			t.Errorf("%s: workbook has %d rows, the CSV bundle has %d", name, len(xlsxRows), len(csvRows))
			continue
		}
		for i, want := range csvRows {
			got := xlsxRows[i]
			for k, wantVal := range want {
				if gotVal := got[k]; gotVal != wantVal {
					t.Errorf("%s row %d column %q: workbook has %q, the CSV bundle has %q "+
						"(if this is a date or a catalog number, the column was not formatted as Text before entry)",
						name, i, k, gotVal, wantVal)
				}
			}
			for k := range got {
				if _, ok := want[k]; !ok {
					t.Errorf("%s row %d: workbook carries column %q the CSV bundle does not", name, i, k)
				}
			}
		}
	}

	// (b) it converts end to end.
	results, err := Convert(fixture, Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("Convert produced no records")
	}

	// (c) a real workbook sits far under every cap.
	zr, err := zip.OpenReader(fixture)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer zr.Close()
	var total, largest int64
	for _, f := range zr.File {
		size := int64(f.UncompressedSize64)
		total += size
		if size > largest {
			largest = size
		}
	}
	if len(zr.File) > maxArchiveEntries/2 {
		t.Errorf("fixture has %d entries, over half the %d limit", len(zr.File), maxArchiveEntries)
	}
	if largest > maxPartInflatedBytes/2 {
		t.Errorf("fixture's largest part is %d bytes, over half the %d per-part limit", largest, maxPartInflatedBytes)
	}
	if total > maxArchiveInflatedBytes/2 {
		t.Errorf("fixture inflates to %d bytes, over half the %d total limit", total, maxArchiveInflatedBytes)
	}
	t.Logf("real Excel fixture: %d entries, largest part %d bytes, %d bytes total inflated", len(zr.File), largest, total)
}

// The ratio gate applies only above a 1 MiB floor. Without the floor a small
// worksheet of repeated inline strings, which routinely deflates past 100:1,
// would be rejected as a bomb. This pins the floor: drop it and this fails.
func TestReadXLSXAcceptsHighRatioBelowTheFloor(t *testing.T) {
	out := filepath.Join(t.TempDir(), "under-floor.xlsx")
	parts := workbookParts("records")
	// 512 KiB of zeros: about 1000:1, far past the ratio limit, but under the
	// 1 MiB floor, so the gate must not look at it.
	parts = append(parts, rawPart{"xl/worksheets/sheet1.xml", worksheetWithPadding(512<<10, zeroPad)})
	writeZipParts(t, out, parts)

	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			if f.UncompressedSize64 >= ratioFloorBytes {
				t.Fatalf("fixture part is %d bytes, must sit under the %d floor", f.UncompressedSize64, ratioFloorBytes)
			}
			if f.CompressedSize64 > 0 && f.UncompressedSize64/f.CompressedSize64 <= maxCompressionRatio {
				t.Fatalf("fixture stores at %d:1, which does not exercise the floor", f.UncompressedSize64/f.CompressedSize64)
			}
		}
	}
	zr.Close()

	if _, err := ReadXLSX(out); err != nil {
		t.Fatalf("a highly compressible part under the ratio floor must be accepted: %v", err)
	}
}

// A zip64 header can declare an uncompressed size that does not fit in int64.
// Converting it before comparing wraps it negative, which passes every gate.
func TestOpenPartRejectsOverflowingClaim(t *testing.T) {
	f := &zip.File{FileHeader: zip.FileHeader{
		Name:               "xl/worksheets/sheet1.xml",
		UncompressedSize64: 1 << 63,
		CompressedSize64:   512,
	}}
	b := newArchiveBudget("fixture.xlsx")
	rc, err := openPart(f, b)
	if rc != nil {
		t.Error("no reader may be returned for a rejected part")
	}
	var lim *ArchiveLimitError
	if !errors.As(err, &lim) {
		t.Fatalf("expected an *ArchiveLimitError, got %v", err)
	}
	if lim.Limit != limitPartBytes {
		t.Errorf("Limit = %q, want %q", lim.Limit, limitPartBytes)
	}
	if lim.Observed <= 0 {
		t.Errorf("Observed = %d, want a positive quantity (a wrapped conversion reads negative)", lim.Observed)
	}
	if b.remaining != maxArchiveInflatedBytes {
		t.Errorf("a rejected part must not spend the budget: remaining = %d", b.remaining)
	}
}

// The size limits bound the bytes a part may inflate; they do not by
// themselves bound the work of turning those bytes into a Workbook. A shared
// string assembled from many small runs is where those two diverge: nothing
// caps the run count, so an accumulator that reallocates per run is quadratic,
// and a part well inside every limit can pin a core for minutes.
//
// The assertion is on allocation volume rather than wall clock, so it does not
// depend on how fast the machine is. Measured on this fixture: about 30x the
// part size when the accumulation is linear, and about 83,000x when it is
// quadratic. Anything near the latter means the accumulator regressed.
func TestReadXLSXSharedStringWithManyRunsIsLinear(t *testing.T) {
	const runs = 400_000
	const maxAllocRatio = 500

	var sst strings.Builder
	sst.WriteString(`<?xml version="1.0"?><sst ` + xlNS + `><si>`)
	for i := 0; i < runs; i++ {
		// Vary the run text: identical runs deflate past 1000:1 and would trip
		// the ratio gate, which would make this a test of rejection instead.
		fmt.Fprintf(&sst, "<r><t>run%07d</t></r>", i)
	}
	sst.WriteString(`</si></sst>`)
	partSize := int64(sst.Len())

	out := filepath.Join(t.TempDir(), "runs.xlsx")
	parts := workbookParts("records")
	for i := range parts {
		if parts[i].name == "xl/sharedStrings.xml" {
			parts[i].data = []byte(sst.String())
		}
	}
	parts = append(parts, rawPart{"xl/worksheets/sheet1.xml", []byte(
		`<?xml version="1.0"?><worksheet ` + xlNS + `><sheetData>` +
			`<row r="1"><c r="A1" t="s"><v>0</v></c></row>` +
			`</sheetData></worksheet>`)})
	writeZipParts(t, out, parts)

	// The part must sit inside the limits, or this would be testing rejection.
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != "xl/sharedStrings.xml" {
			continue
		}
		if f.UncompressedSize64 > maxPartInflatedBytes {
			t.Fatalf("fixture part is %d bytes, over the %d per-part limit", f.UncompressedSize64, maxPartInflatedBytes)
		}
		if f.CompressedSize64 > 0 && f.UncompressedSize64/f.CompressedSize64 > maxCompressionRatio {
			t.Fatalf("fixture stores at %d:1, which would trip the ratio gate instead", f.UncompressedSize64/f.CompressedSize64)
		}
	}
	zr.Close()

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	wb, err := ReadXLSX(out)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("ReadXLSX: %v", err)
	}
	if _, ok := wb["records"]; !ok {
		t.Errorf("workbook has no records sheet")
	}

	ratio := float64(after.TotalAlloc-before.TotalAlloc) / float64(partSize)
	t.Logf("part %d bytes, allocated %.1fx the part size", partSize, ratio)
	if ratio > maxAllocRatio {
		t.Errorf("reading a %d-byte part allocated %.0fx its size (limit %dx): the shared-string accumulator is superlinear in the run count",
			partSize, ratio, maxAllocRatio)
	}
}
