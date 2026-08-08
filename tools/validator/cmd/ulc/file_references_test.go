package main

import (
	"bytes"
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
