package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The vode record dual-writes its evidence document: the .ies the first
// attestation references is also listed in source_files with an identical
// filename and hash, which is what the shipped authoring guidance instructs.
// That shape makes a naive fixture vacuous, because the default source_files
// walk already errors on a tampered file with or without the flag. Every
// fixture below therefore drops the dual-written source_files entry first and
// repairs index.source_file_types_present, so what it asserts is the new site
// and nothing else.
const vodeEvidenceFile = "807-nexa3-35k-90cri-so-hl-bl.ies"
const vodeCutsheetFile = "vode-nexa-suspended-hl-rg-bl-specsheet-feb2026.pdf"

// recordFixture copies an example record into a fresh temp directory, applies
// mutate to the parsed tree, and returns the path of the copy. The repo's
// examples stay read-only. Numbers round-trip through json.Number so the copy
// keeps the original's numeric spelling and builder parity is unaffected.
func recordFixture(t *testing.T, name string, mutate func(rec map[string]any)) string {
	t.Helper()
	data, err := os.ReadFile(exampleRecord(t, name))
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var rec map[string]any
	if err := dec.Decode(&rec); err != nil {
		t.Fatalf("parse example: %v", err)
	}
	mutate(rec)
	out := filepath.Join(t.TempDir(), name)
	encoded, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	if err := os.WriteFile(out, encoded, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return out
}

// dropSourceFile removes the source_files entry naming filename and rewrites
// index.source_file_types_present to the types that remain, which is what the
// builder projects. Dropping the entry without the index repair yields
// ERROR index/drift and the fixture would assert nothing about hashes.
func dropSourceFile(t *testing.T, rec map[string]any, filename string) {
	t.Helper()
	arr, ok := rec["source_files"].([]any)
	if !ok {
		t.Fatalf("record has no source_files array")
	}
	kept := make([]any, 0, len(arr))
	types := map[string]bool{}
	removed := false
	for _, entry := range arr {
		m := entry.(map[string]any)
		ref := m["reference"].(map[string]any)
		if ref["filename"] == filename {
			removed = true
			continue
		}
		kept = append(kept, entry)
		types[m["file_type"].(string)] = true
	}
	if !removed {
		t.Fatalf("no source_files entry names %q", filename)
	}
	rec["source_files"] = kept

	present := make([]string, 0, len(types))
	for ty := range types {
		present = append(present, ty)
	}
	sort.Strings(present)
	asAny := make([]any, len(present))
	for i, ty := range present {
		asAny[i] = ty
	}
	rec["index"].(map[string]any)["source_file_types_present"] = asAny
}

func writeBeside(t *testing.T, recordPath, filename string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(filepath.Dir(recordPath), filename), content, 0o644); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}

// TestCLIVerifyEvidenceTamperedDocument pins the flag's whole point: an
// evidence document that the record references but does not list among its
// source files is unchecked by default and an ERROR under the flag.
func TestCLIVerifyEvidenceTamperedDocument(t *testing.T) {
	rec := recordFixture(t, vodeRecord, func(rec map[string]any) {
		dropSourceFile(t, rec, vodeEvidenceFile)
	})
	writeBeside(t, rec, vodeEvidenceFile, []byte("not the evidence bytes"))

	if code := runValidate([]string{rec}); code != 0 {
		t.Fatalf("default run exit = %d, want 0 (the evidence site is not checked by default)", code)
	}

	out, code := captureStdout(t, func() int {
		return runValidate([]string{rec, "--verify-evidence"})
	})
	if code != 1 {
		t.Fatalf("--verify-evidence exit = %d, want 1 (a local mismatch is an ERROR)", code)
	}
	if !strings.Contains(out, "ERROR") || !strings.Contains(out, "source-file/hash-mismatch at /attestations/0/source_document_ref") {
		t.Errorf("missing hash-mismatch ERROR at /attestations/0/source_document_ref; got:\n%s", out)
	}
}

// TestCLIVerifyEvidenceAbsentDocumentStaysInfo pins the withheld-evidence
// pattern: a record that publishes the reference while withholding the
// document still validates, flag or no flag.
func TestCLIVerifyEvidenceAbsentDocumentStaysInfo(t *testing.T) {
	rec := recordFixture(t, vodeRecord, func(rec map[string]any) {
		dropSourceFile(t, rec, vodeEvidenceFile)
	})

	out, code := captureStdout(t, func() int {
		return runValidate([]string{rec, "--verify-evidence"})
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (an absent evidence document stays INFO)", code)
	}
	if !strings.Contains(out, "source-file/not-found-locally at /attestations/0/source_document_ref") {
		t.Errorf("missing not-found INFO at /attestations/0/source_document_ref; got:\n%s", out)
	}
}

// TestCLIVerifyEvidenceFlagOrderAndJSON confirms the boolean flag rides
// through reorderFlagsFirst in either argument order and composes with --json.
func TestCLIVerifyEvidenceFlagOrderAndJSON(t *testing.T) {
	rec := recordFixture(t, vodeRecord, func(rec map[string]any) {
		dropSourceFile(t, rec, vodeEvidenceFile)
	})
	writeBeside(t, rec, vodeEvidenceFile, []byte("not the evidence bytes"))

	for _, args := range [][]string{
		{rec, "--verify-evidence"},
		{"--verify-evidence", rec},
	} {
		if code := runValidate(args); code != 1 {
			t.Errorf("args %v: exit = %d, want 1", args, code)
		}
	}

	out, code := captureStdout(t, func() int {
		return runValidate([]string{rec, "--verify-evidence", "--json"})
	})
	if code != 1 {
		t.Fatalf("--verify-evidence --json exit = %d, want 1", code)
	}
	var envelope struct {
		Findings []struct {
			Level string `json:"level"`
			Code  string `json:"code"`
			Path  string `json:"path"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("parse JSON output: %v", err)
	}
	found := false
	for _, f := range envelope.Findings {
		if f.Path == "/attestations/0/source_document_ref" && f.Code == "source-file/hash-mismatch" {
			found = true
		}
	}
	if !found {
		t.Errorf("JSON output carries no hash-mismatch at /attestations/0/source_document_ref; got:\n%s", out)
	}
}

// TestCLICutsheetHashMismatch pins the new default site. The cutsheet is
// dual-written on every shipped example, so the fixture drops the datasheet
// source_files entry first; without that surgery this passes on the old
// binary too and pins nothing.
func TestCLICutsheetHashMismatch(t *testing.T) {
	rec := recordFixture(t, vodeRecord, func(rec map[string]any) {
		dropSourceFile(t, rec, vodeCutsheetFile)
	})
	writeBeside(t, rec, vodeCutsheetFile, []byte("not the datasheet bytes"))

	out, code := captureStdout(t, func() int { return runValidate([]string{rec}) })
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (a tampered cutsheet is an ERROR on a default run)", code)
	}
	if !strings.Contains(out, "source-file/hash-mismatch at /product_family/cutsheet") {
		t.Errorf("missing hash-mismatch ERROR at /product_family/cutsheet; got:\n%s", out)
	}
}

// TestCLICutsheetMismatchOnDualWrittenRecord covers the pre-existing
// source_files path, not the new site: on an unmodified example the cutsheet
// is also a source file, so a tampered file fails validation the same way it
// did before this release. Keeping the two cases apart is the point.
func TestCLICutsheetMismatchOnDualWrittenRecord(t *testing.T) {
	rec := recordFixture(t, vodeRecord, func(rec map[string]any) {})
	writeBeside(t, rec, vodeCutsheetFile, []byte("not the datasheet bytes"))

	out, code := captureStdout(t, func() int { return runValidate([]string{rec}) })
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(out, "source-file/hash-mismatch at /source_files/0/reference") {
		t.Errorf("missing the pre-existing source_files ERROR; got:\n%s", out)
	}
}

// TestCLIFromSheetSanitizesRecordControlledOutput pins the from-sheet render
// sites. The record id comes from a workbook cell, which is untrusted input
// that reaches stdout before any schema validation runs, so a crafted one
// could otherwise forge a report line or drive the terminal. Every echo of it
// goes through the sanitizer; drop any one wrapper and this fails.
func TestCLIFromSheetSanitizesRecordControlledOutput(t *testing.T) {
	src := filepath.Join(repoRoot(t), "tools", "validator", "internal", "sheet", "testdata", "bundle")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("bundle fixture not available: %v", err)
	}

	// An ESC sequence, a DEL, and a newline followed by a forged success line.
	const hostile = "acme\x1b]0;pwned\x07\x7f\nOK -- forged: 0 errors, 0 warnings, 0 infos.-1200"

	// Copy the bundle so the repo fixture stays read-only. The CSVs are
	// rewritten through encoding/csv rather than by string substitution, so the
	// embedded newline and control bytes are correctly quoted and the converter
	// reaches the record-id echo sites instead of failing on a malformed field.
	bundle := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	rewrote := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if filepath.Ext(e.Name()) == ".csv" {
			rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
			if err != nil {
				t.Fatalf("parse %s: %v", e.Name(), err)
			}
			// record_id is the join key and is the first column of every sheet
			// in this bundle; rewrite it by header name in each.
			col := -1
			for i, h := range rows[0] {
				if h == "record_id" {
					col = i
				}
			}
			for _, row := range rows[1:] {
				if col >= 0 && col < len(row) {
					row[col] = hostile
					rewrote = true
				}
			}
			var buf bytes.Buffer
			w := csv.NewWriter(&buf)
			if err := w.WriteAll(rows); err != nil {
				t.Fatalf("write %s: %v", e.Name(), err)
			}
			data = buf.Bytes()
		}
		if err := os.WriteFile(filepath.Join(bundle, e.Name()), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", e.Name(), err)
		}
	}
	if !rewrote {
		t.Fatal("no cell carried the record id, so nothing hostile was planted")
	}

	out, _ := captureStdout(t, func() int {
		return runFromSheet([]string{"--out", t.TempDir(), "--allow-missing-files", bundle})
	})

	// The converter must have gotten far enough to echo the record id; a parse
	// failure would make the control-byte assertion below vacuous.
	if !strings.Contains(out, "acme") {
		t.Fatalf("from-sheet never echoed the record id, so this asserts nothing:\n%q", out)
	}
	for i := 0; i < len(out); i++ {
		if b := out[i]; b != '\n' && (b < 0x20 || b == 0x7F) {
			t.Fatalf("control byte %#x reached stdout at offset %d:\n%q", b, i, out)
		}
	}
	// The forged text may appear (escaping makes it visible, not invisible);
	// what must not happen is it starting a line of its own.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "OK -- forged:") {
			t.Errorf("a forged report line survived rendering:\n%q", out)
		}
	}
	if !strings.Contains(out, `\x0A`) {
		t.Errorf("the newline in the record id should render as a visible escape:\n%q", out)
	}
}
