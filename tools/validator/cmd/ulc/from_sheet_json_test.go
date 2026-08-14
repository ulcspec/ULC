package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sheetBundle copies the reference CSV bundle into a temp dir so a test can
// mutate it without touching the repo fixture.
func sheetBundle(t *testing.T) string {
	t.Helper()
	src := filepath.Join(repoRoot(t), "tools", "validator", "internal", "sheet", "testdata", "bundle")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("bundle fixture not available: %v", err)
	}
	dst := t.TempDir()
	copyFlatDir(t, src, dst)
	return dst
}

// rewriteRecordsCell replaces one records.csv cell by header name, going
// through encoding/csv so a value carrying a comma, a quote, or a control byte
// is quoted correctly rather than reshaping the row.
func rewriteRecordsCell(t *testing.T, bundle, header, value string) {
	t.Helper()
	path := filepath.Join(bundle, "records.csv")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read records.csv: %v", err)
	}
	rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		t.Fatalf("parse records.csv: %v", err)
	}
	col := -1
	for i, h := range rows[0] {
		if h == header {
			col = i
		}
	}
	if col < 0 {
		t.Fatalf("records.csv has no %q column", header)
	}
	for _, row := range rows[1:] {
		row[col] = value
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.WriteAll(rows); err != nil {
		t.Fatalf("write records.csv: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write records.csv: %v", err)
	}
}

// rewriteRecordIDEverywhere sets record_id, the join key present in every sheet
// of the bundle, to the given value across all CSVs.
func rewriteRecordIDEverywhere(t *testing.T, bundle, value string) {
	t.Helper()
	entries, err := os.ReadDir(bundle)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	rewrote := false
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".csv" {
			continue
		}
		path := filepath.Join(bundle, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		col := -1
		for i, h := range rows[0] {
			if h == "record_id" {
				col = i
			}
		}
		for _, row := range rows[1:] {
			if col >= 0 && col < len(row) {
				row[col] = value
				rewrote = true
			}
		}
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		if err := w.WriteAll(rows); err != nil {
			t.Fatalf("write %s: %v", e.Name(), err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatalf("write %s: %v", e.Name(), err)
		}
	}
	if !rewrote {
		t.Fatal("no cell carried the record id, so nothing was planted")
	}
}

// jsonReport is the decode target for the --json conversion report. It restates
// the wire contract rather than reusing the emitting structs, so a rename on
// either side shows up as a test failure.
type jsonReport struct {
	FromSheetVersion string `json:"from_sheet_version"`
	CLIVersion       string `json:"cli_version"`
	Records          []struct {
		RecordID         string   `json:"record_id"`
		Pattern          string   `json:"pattern"`
		Status           string   `json:"status"`
		ConformanceLevel string   `json:"conformance_level"`
		Path             string   `json:"path"`
		DraftPath        string   `json:"draft_path"`
		Warnings         []string `json:"warnings"`
		Error            string   `json:"error"`
		Findings         *struct {
			Summary map[string]any `json:"summary"`
		} `json:"findings"`
	} `json:"records"`
	Summary struct {
		Written int `json:"written"`
		Drafts  int `json:"drafts"`
		Failed  int `json:"failed"`
	} `json:"summary"`
}

func decodeReport(t *testing.T, out string) jsonReport {
	t.Helper()
	var doc jsonReport
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("stdout is not the JSON report: %v\n%s", err, out)
	}
	return doc
}

// countULCRecords counts written records in an --out directory.
func countULCRecords(t *testing.T, dir string) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.ulc.json"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	return len(matches)
}

// TestCLIFromSheetJSONReport pins the --json conversion report on the happy
// path, and pins the escaping the contract promises for the untrusted,
// workbook-controlled record_id.
func TestCLIFromSheetJSONReport(t *testing.T) {
	bundle := sheetBundle(t)
	outDir := t.TempDir()

	out, code := captureStdout(t, func() int {
		return runFromSheet([]string{"--out", outDir, "--json", bundle})
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\n%s", code, out)
	}

	doc := decodeReport(t, out)
	if doc.FromSheetVersion != "1.0.0" {
		t.Errorf("from_sheet_version = %q, want 1.0.0", doc.FromSheetVersion)
	}
	if doc.CLIVersion != CLIVersion {
		t.Errorf("cli_version = %q, want %q", doc.CLIVersion, CLIVersion)
	}
	if len(doc.Records) != 1 {
		t.Fatalf("got %d records, want 1", len(doc.Records))
	}
	rec := doc.Records[0]
	if rec.RecordID != "acme-orbit-1200-4000k" {
		t.Errorf("record_id = %q", rec.RecordID)
	}
	if rec.Pattern != "A" {
		t.Errorf("pattern = %q, want A", rec.Pattern)
	}
	if rec.Status != "written" {
		t.Errorf("status = %q, want written", rec.Status)
	}
	if rec.ConformanceLevel == "" {
		t.Error("conformance_level must be populated on every entry")
	}
	if rec.Path == "" {
		t.Error("path must name the written record")
	}
	if rec.Warnings == nil {
		t.Error("warnings must serialize as an array, never null")
	}
	if rec.Findings == nil || rec.Findings.Summary == nil {
		t.Error("findings must carry the {findings, summary} report for a validated record")
	}
	if doc.Summary.Written != 1 || doc.Summary.Drafts != 0 || doc.Summary.Failed != 0 {
		t.Errorf("summary = %+v, want 1 written", doc.Summary)
	}

	// The JSON path's escaping is printJSON, a different mechanism from the
	// text path's SanitizeText, so it needs its own proof: the decoded value
	// round-trips the hostile bytes exactly, while the raw stdout carries none
	// of them.
	const hostile = "acme\x1b]0;pwned\x07\x7f\n<script>&amp;OK -- forged: 0 errors, 0 warnings, 0 infos.-1200"
	hostileBundle := sheetBundle(t)
	rewriteRecordIDEverywhere(t, hostileBundle, hostile)

	raw, _ := captureStdout(t, func() int {
		return runFromSheet([]string{"--out", t.TempDir(), "--json", hostileBundle})
	})
	hostileDoc := decodeReport(t, raw)
	if len(hostileDoc.Records) != 1 {
		t.Fatalf("got %d records for the hostile fixture, want 1", len(hostileDoc.Records))
	}
	if got := hostileDoc.Records[0].RecordID; got != hostile {
		// Non-vacuity: the id must have reached the report at all.
		t.Fatalf("decoded record_id = %q, want the hostile bytes round-tripped", got)
	}
	// encoding/json escapes every byte below 0x20, so none survives into the
	// document's own bytes; the newlines present are the encoder's own
	// formatting.
	for i := 0; i < len(raw); i++ {
		if b := raw[i]; b != '\n' && b < 0x20 {
			t.Fatalf("control byte %#x reached raw stdout at offset %d:\n%q", b, i, raw)
		}
	}
	for _, c := range []string{"<", ">", "&"} {
		if strings.Contains(raw, c) {
			t.Errorf("raw stdout carries an unescaped %q; the report must escape it like ulc scope", c)
		}
	}
}

// TestCLIFromSheetJSONDraftAndDraftOut pins the draft disposition: --draft-out
// saves a draft under its own suffix and never into --out, the default text
// output is unchanged, and the draft-filename guard refuses an unsafe
// record_id.
func TestCLIFromSheetJSONDraftAndDraftOut(t *testing.T) {
	// Leg 1: --draft-out --json.
	bundle := sheetBundle(t)
	rewriteRecordsCell(t, bundle, "cutsheet_file", "not-in-the-bundle.pdf")
	outDir, draftDir := t.TempDir(), t.TempDir()

	out, code := captureStdout(t, func() int {
		return runFromSheet([]string{"--out", outDir, "--allow-missing-files", "--draft-out", draftDir, "--json", bundle})
	})
	if code != 1 {
		t.Errorf("exit code = %d, want 1 (a draft is not a success)", code)
	}
	doc := decodeReport(t, out)
	if len(doc.Records) != 1 {
		t.Fatalf("got %d records, want 1", len(doc.Records))
	}
	rec := doc.Records[0]
	if rec.Status != "draft" {
		t.Errorf("status = %q, want draft", rec.Status)
	}
	if rec.ConformanceLevel == "" {
		t.Error("a draft entry must still carry conformance_level")
	}
	wantDraft := filepath.Join(draftDir, "acme-orbit-1200-4000k.draft.json")
	if rec.DraftPath != wantDraft {
		t.Errorf("draft_path = %q, want %q", rec.DraftPath, wantDraft)
	}
	if doc.Summary.Drafts != 1 {
		t.Errorf("summary = %+v, want 1 draft", doc.Summary)
	}
	draftBytes, err := os.ReadFile(wantDraft)
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	var draft map[string]any
	if err := json.Unmarshal(draftBytes, &draft); err != nil {
		t.Fatalf("draft is not JSON: %v", err)
	}
	const placeholder = "0000000000000000000000000000000000000000000000000000000000000000"
	family, _ := draft["product_family"].(map[string]any)
	cutsheet, _ := family["cutsheet"].(map[string]any)
	if got, _ := cutsheet["sha256"].(string); got != placeholder {
		t.Errorf("draft cutsheet sha256 = %q, want the 64-zero placeholder", got)
	}
	if n := countULCRecords(t, outDir); n != 0 {
		t.Errorf("--out holds %d records; a draft must never be written there", n)
	}

	// Leg 2: no --draft-out, no --json. The shipped text output is unchanged.
	plainBundle := sheetBundle(t)
	rewriteRecordsCell(t, plainBundle, "cutsheet_file", "not-in-the-bundle.pdf")
	plainOut := t.TempDir()

	stdout, stderr, code := captureOutErr(t, func() int {
		return runFromSheet([]string{"--out", plainOut, "--allow-missing-files", plainBundle})
	})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	const wantLine = "acme-orbit-1200-4000k -> DRAFT, not written (references files not present; --allow-missing-files stamped placeholder hashes)\n"
	if stdout != wantLine {
		t.Errorf("stdout = %q, want %q", stdout, wantLine)
	}
	if !strings.Contains(stderr, "warning: acme-orbit-1200-4000k: file \"not-in-the-bundle.pdf\" not found") {
		t.Errorf("stderr does not carry the missing-file warning:\n%s", stderr)
	}
	const wantNotice = "ulc from-sheet: one or more records reference files that were not hashed (--allow-missing-files stamped the zero-sentinel SHA-256). These are DRAFTS, not validated records; re-run without --allow-missing-files once the files are present.\n"
	if !strings.Contains(stderr, wantNotice) {
		t.Errorf("stderr does not carry the drafts notice verbatim:\n%s", stderr)
	}

	// Leg 3: the draft-filename guard. The guard lives inside the sentinel
	// branch, so the fixture must BOTH stamp a sentinel and carry a hostile id,
	// or the leg proves nothing.
	guardBundle := sheetBundle(t)
	rewriteRecordsCell(t, guardBundle, "cutsheet_file", "not-in-the-bundle.pdf")
	rewriteRecordIDEverywhere(t, guardBundle, "../escaped")
	guardOut, guardDrafts := t.TempDir(), t.TempDir()

	gStdout, gStderr, code := captureOutErr(t, func() int {
		return runFromSheet([]string{"--out", guardOut, "--allow-missing-files", "--draft-out", guardDrafts, "--json", guardBundle})
	})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	guardDoc := decodeReport(t, gStdout)
	if len(guardDoc.Records) != 1 {
		t.Fatalf("got %d records, want 1", len(guardDoc.Records))
	}
	if got := guardDoc.Records[0].Status; got != "failed" {
		t.Errorf("status = %q, want failed", got)
	}
	if got := guardDoc.Records[0].Error; got != "unsafe record_id for draft filename" {
		t.Errorf("error = %q, want the unsafe-record_id message", got)
	}
	if !strings.Contains(gStderr, "record_id is not a safe filename component") {
		t.Errorf("stderr does not name the guard:\n%s", gStderr)
	}
	entries, err := os.ReadDir(guardDrafts)
	if err != nil {
		t.Fatalf("read draft dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("draft dir holds %d entries, want none", len(entries))
	}
	// Nothing escaped the draft directory either.
	if _, err := os.Stat(filepath.Join(filepath.Dir(guardDrafts), "escaped.draft.json")); err == nil {
		t.Error("a draft was written outside the draft directory")
	}
}

// TestCLIFromSheetRefusesToWriteInvalidRecord pins the refusal-to-write path: a
// record that fails schema validation is reported, is not written, and the run
// exits 1.
func TestCLIFromSheetRefusesToWriteInvalidRecord(t *testing.T) {
	bundle := sheetBundle(t)
	// The fixture header does not carry record_status_as_of, so the column is
	// appended rather than substituted. A malformed date fails the format
	// assertion with one ERROR at /record_status_as_of.
	path := filepath.Join(bundle, "records.csv")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read records.csv: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected a header and one row, got %d lines", len(lines))
	}
	lines[0] += ",record_status_as_of"
	lines[1] += ",3/1/26"
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write records.csv: %v", err)
	}

	outDir := t.TempDir()
	stdout, _, code := captureOutErr(t, func() int {
		return runFromSheet([]string{"--out", outDir, bundle})
	})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout, "not written") {
		t.Errorf("stdout does not report the refusal:\n%s", stdout)
	}
	if n := countULCRecords(t, outDir); n != 0 {
		t.Errorf("--out holds %d records; a record failing validation must not be written", n)
	}
}

// TestCLIFromSheetRejectsOversizedArchive pins archive-cap propagation at the
// CLI, and pins that a workbook-level conversion error produces no JSON
// document.
func TestCLIFromSheetRejectsOversizedArchive(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "over-cap.xlsx")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	zw := zip.NewWriter(f)
	// The entry-count pre-check fires immediately after the archive opens,
	// before any part is parsed, so tiny filler entries suffice.
	for i := 0; i <= 1024; i++ {
		w, err := zw.Create(fmt.Sprintf("filler/%04d.bin", i))
		if err != nil {
			t.Fatalf("zip create: %v", err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	_, stderr, code := captureOutErr(t, func() int {
		return runFromSheet([]string{"--out", t.TempDir(), archive})
	})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "the limit is 1024") {
		t.Errorf("stderr does not name the limit:\n%s", stderr)
	}

	// With --json, a workbook-level failure still produces no document.
	stdout, stderr, code := captureOutErr(t, func() int {
		return runFromSheet([]string{"--out", t.TempDir(), "--json", archive})
	})
	if code != 1 {
		t.Errorf("exit code with --json = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty: a conversion error produces no JSON document", stdout)
	}
	if !strings.Contains(stderr, "the limit is 1024") {
		t.Errorf("stderr with --json does not name the limit:\n%s", stderr)
	}
}
