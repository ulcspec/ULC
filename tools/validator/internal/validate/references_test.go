package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// siteFixture places one FileReference at one registry site and states the
// runtime pointer the walk must report it at. Every site in
// fileReferenceRegistry has an entry here, so the outcome matrix below runs
// against all seven rather than against source_files alone.
type siteFixture struct {
	name    string
	pointer string
	build   func(ref map[string]any) map[string]any
}

var siteFixtures = []siteFixture{
	{
		name:    "source_files",
		pointer: "/source_files/0/reference",
		build: func(ref map[string]any) map[string]any {
			return map[string]any{"source_files": []any{
				map[string]any{"file_type": "datasheet_pdf", "reference": ref},
			}}
		},
	},
	{
		name:    "cutsheet",
		pointer: "/product_family/cutsheet",
		build: func(ref map[string]any) map[string]any {
			return map[string]any{"product_family": map[string]any{"cutsheet": ref}}
		},
	},
	{
		name:    "emergency_photometry",
		pointer: "/emergency/photometry_reference",
		build: func(ref map[string]any) map[string]any {
			return map[string]any{"emergency": map[string]any{"photometry_reference": ref}}
		},
	},
	{
		name:    "warranty_conditions",
		pointer: "/product_family/shared_warranty/conditions_document",
		build: func(ref map[string]any) map[string]any {
			return map[string]any{"product_family": map[string]any{
				"shared_warranty": map[string]any{"conditions_document": ref},
			}}
		},
	},
	{
		name:    "media",
		pointer: "/media/0/reference",
		build: func(ref map[string]any) map[string]any {
			return map[string]any{"media": []any{
				map[string]any{"role": "product_photo", "media_type": "image/jpeg", "reference": ref},
			}}
		},
	},
	{
		name:    "attestation_evidence",
		pointer: "/attestations/0/source_document_ref",
		build: func(ref map[string]any) map[string]any {
			return map[string]any{"attestations": []any{
				map[string]any{"theme": "emergency", "source_document_ref": ref},
			}}
		},
	},
	{
		name:    "shared_attestation_evidence",
		pointer: "/product_family/shared_attestations/0/source_document_ref",
		build: func(ref map[string]any) map[string]any {
			return map[string]any{"product_family": map[string]any{
				"shared_attestations": []any{
					map[string]any{"theme": "material_health", "source_document_ref": ref},
				},
			}}
		},
	},
}

// TestVerifyFileReferencesCoversEveryFixture keeps the fixture table honest: a
// registry row added without a fixture would silently drop out of the outcome
// matrix below. Matching on the site set rather than the count also catches a
// duplicated fixture, which would keep the counts equal while leaving the new
// site unexercised.
func TestVerifyFileReferencesCoversEveryFixture(t *testing.T) {
	want := map[string]bool{}
	for _, site := range fileReferenceRegistry {
		want[strings.Replace(site.family, "<i>", "0", 1)] = true
	}
	got := map[string]bool{}
	for _, f := range siteFixtures {
		if got[f.pointer] {
			t.Errorf("two fixtures share the pointer %q", f.pointer)
		}
		got[f.pointer] = true
	}
	for p := range want {
		if !got[p] {
			t.Errorf("registry site %q has no fixture, so no outcome is exercised there", p)
		}
	}
	for p := range got {
		if !want[p] {
			t.Errorf("fixture %q names no registry site", p)
		}
	}
}

// TestFileReferenceRegistryPolicies pins each site's policy by family name.
// Without this, flipping a default site to evidence-only passes the whole
// suite: the outcome matrix runs under Evidence: true, the drift guard only
// rejects policyUnset, and no shipped example carries an
// emergency.photometry_reference for the corpus run to catch it.
func TestFileReferenceRegistryPolicies(t *testing.T) {
	want := map[string]refPolicy{
		"/source_files/<i>/reference":                                 policyDefault,
		"/product_family/cutsheet":                                    policyDefault,
		"/emergency/photometry_reference":                             policyDefault,
		"/product_family/shared_warranty/conditions_document":         policyDefault,
		"/media/<i>/reference":                                        policyDefault,
		"/attestations/<i>/source_document_ref":                       policyEvidence,
		"/product_family/shared_attestations/<i>/source_document_ref": policyEvidence,
	}
	if len(fileReferenceRegistry) != len(want) {
		t.Fatalf("registry has %d rows, this test pins %d", len(fileReferenceRegistry), len(want))
	}
	for _, site := range fileReferenceRegistry {
		w, ok := want[site.family]
		if !ok {
			t.Errorf("registry names %q, which this test does not pin", site.family)
			continue
		}
		if site.policy != w {
			t.Errorf("%s: policy = %d, want %d (changing a site's policy changes default output and must be a deliberate, released decision)", site.family, site.policy, w)
		}
	}
}

// TestVerifyFileReferencesDefaultSitesRunWithoutTheFlag is the runtime half of
// the policy pin: the five default sites report on a plain run.
func TestVerifyFileReferencesDefaultSitesRunWithoutTheFlag(t *testing.T) {
	for _, site := range siteFixtures {
		policy := refPolicy(policyUnset)
		for _, row := range fileReferenceRegistry {
			if strings.Replace(row.family, "<i>", "0", 1) == site.pointer {
				policy = row.policy
			}
		}
		if policy != policyDefault {
			continue
		}
		t.Run(site.name, func(t *testing.T) {
			record := site.build(map[string]any{
				"filename": "missing.pdf", "sha256": strings.Repeat("b", 64),
			})
			report := findings.NewReport()
			VerifyFileReferences(t.TempDir(), record, VerifyOptions{}, report)
			one(t, report.Findings, findings.LevelInfo, findings.CodeSourceFileNotFound, site.pointer, "not present locally")
		})
	}
}

// TestVerifyFileReferencesOutcomesPerSite runs every outcome verifyOne can
// produce against every registry site, under Evidence: true so the attestation
// homes are exercised too (the Evidence-off gating is asserted separately, in
// TestVerifyFileReferencesEvidenceGating). Semantics must be identical at
// every site; only the reported pointer differs.
func TestVerifyFileReferencesOutcomesPerSite(t *testing.T) {
	outcomes := []struct {
		name  string
		ref   func(t *testing.T, outer, recordDir string) map[string]any
		check func(t *testing.T, pointer string, got []findings.Finding)
	}{
		{
			name: "present_and_matching_is_silent",
			ref: func(t *testing.T, outer, recordDir string) map[string]any {
				content := []byte("hello ulc")
				writeFile(t, filepath.Join(recordDir, "match.pdf"), content)
				return map[string]any{"filename": "match.pdf", "sha256": sha256Hex(content)}
			},
			check: func(t *testing.T, pointer string, got []findings.Finding) {
				if len(got) != 0 {
					t.Fatalf("expected no finding for a matching file, got %v", got)
				}
			},
		},
		{
			name: "present_and_mismatching_is_error",
			ref: func(t *testing.T, outer, recordDir string) map[string]any {
				writeFile(t, filepath.Join(recordDir, "tampered.pdf"), []byte("some bytes"))
				return map[string]any{"filename": "tampered.pdf", "sha256": sha256Hex([]byte("different bytes"))}
			},
			check: func(t *testing.T, pointer string, got []findings.Finding) {
				one(t, got, findings.LevelError, findings.CodeSourceFileHashMismatch, pointer, "mismatch")
			},
		},
		{
			name: "absent_is_info",
			ref: func(t *testing.T, outer, recordDir string) map[string]any {
				return map[string]any{"filename": "missing.ies", "sha256": sha256Hex([]byte("irrelevant"))}
			},
			check: func(t *testing.T, pointer string, got []findings.Finding) {
				one(t, got, findings.LevelInfo, findings.CodeSourceFileNotFound, pointer, "not present locally")
			},
		},
		{
			name: "absolute_path_is_contained",
			ref: func(t *testing.T, outer, recordDir string) map[string]any {
				secret := filepath.Join(outer, "secret.txt")
				writeFile(t, secret, []byte("top secret"))
				return map[string]any{"filename": secret, "sha256": strings.Repeat("a", 64)}
			},
			check: func(t *testing.T, pointer string, got []findings.Finding) {
				one(t, got, findings.LevelInfo, findings.CodeSourceFileNotFound, pointer, "is absolute")
			},
		},
		{
			name: "traversal_is_contained",
			ref: func(t *testing.T, outer, recordDir string) map[string]any {
				writeFile(t, filepath.Join(outer, "secret.txt"), []byte("top secret"))
				return map[string]any{"filename": "../secret.txt", "sha256": strings.Repeat("a", 64)}
			},
			check: func(t *testing.T, pointer string, got []findings.Finding) {
				one(t, got, findings.LevelInfo, findings.CodeSourceFileNotFound, pointer, "outside the record directory")
			},
		},
		{
			name: "symlink_escape_is_contained",
			ref: func(t *testing.T, outer, recordDir string) map[string]any {
				target := filepath.Join(outer, "target.txt")
				writeFile(t, target, []byte("sensitive"))
				link := filepath.Join(recordDir, "looks-innocent.pdf")
				if err := os.Symlink(target, link); err != nil {
					t.Skipf("symlink not supported on this platform: %v", err)
				}
				return map[string]any{"filename": "looks-innocent.pdf", "sha256": strings.Repeat("a", 64)}
			},
			check: func(t *testing.T, pointer string, got []findings.Finding) {
				one(t, got, findings.LevelInfo, findings.CodeSourceFileNotFound, pointer, "symlink")
			},
		},
		{
			name: "unreadable_is_warning",
			ref: func(t *testing.T, outer, recordDir string) map[string]any {
				// A directory opens but cannot be read, which is the read-failure
				// branch. Unlike chmod 0000 it behaves the same for any uid, so
				// this case does not go quiet when the suite runs as root.
				if err := os.MkdirAll(filepath.Join(recordDir, "not-a-file.pdf"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				return map[string]any{"filename": "not-a-file.pdf", "sha256": strings.Repeat("a", 64)}
			},
			check: func(t *testing.T, pointer string, got []findings.Finding) {
				one(t, got, findings.LevelWarning, findings.CodeSourceFileUnreadable, pointer, "could not read")
			},
		},
	}

	for _, site := range siteFixtures {
		for _, oc := range outcomes {
			t.Run(site.name+"/"+oc.name, func(t *testing.T) {
				outer := t.TempDir()
				recordDir := filepath.Join(outer, "records")
				if err := os.MkdirAll(recordDir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				record := site.build(oc.ref(t, outer, recordDir))
				report := findings.NewReport()
				VerifyFileReferences(recordDir, record, VerifyOptions{Evidence: true}, report)
				oc.check(t, site.pointer, report.Findings)
			})
		}
	}
}

// TestVerifyFileReferencesSkipsNonMapNodes confirms every site tolerates a
// node of the wrong shape without reporting: schema validation owns complaints
// about shape, and the walk runs on schema-invalid records too.
func TestVerifyFileReferencesSkipsNonMapNodes(t *testing.T) {
	records := []map[string]any{
		{"source_files": "not an array"},
		{"source_files": []any{"not a map"}},
		{"source_files": []any{map[string]any{"reference": "not a map"}}},
		{"product_family": "not a map"},
		{"product_family": map[string]any{"cutsheet": []any{}}},
		{"emergency": map[string]any{"photometry_reference": 42}},
		// The warranty-conditions site is the only one with two intermediate
		// object hops, so a wrong shape at each hop is exercised.
		{"product_family": map[string]any{"shared_warranty": "not a map"}},
		{"product_family": map[string]any{"shared_warranty": map[string]any{"conditions_document": 42}}},
		{"attestations": map[string]any{"not": "an array"}},
		{"attestations": []any{map[string]any{"source_document_ref": nil}}},
		{"product_family": map[string]any{"shared_attestations": []any{7}}},
	}
	for _, record := range records {
		report := findings.NewReport()
		VerifyFileReferences(t.TempDir(), record, VerifyOptions{Evidence: true}, report)
		if len(report.Findings) != 0 {
			t.Errorf("expected no findings for %v, got %v", record, report.Findings)
		}
	}
}

// TestVerifyFileReferencesEvidenceGating pins the opt-in contract: the
// attestation evidence homes are silent on a default run even when a local
// document mismatches its declared hash, and are fully checked under the flag.
func TestVerifyFileReferencesEvidenceGating(t *testing.T) {
	recordDir := t.TempDir()
	writeFile(t, filepath.Join(recordDir, "evidence.pdf"), []byte("real bytes"))
	ref := func() map[string]any {
		return map[string]any{"filename": "evidence.pdf", "sha256": sha256Hex([]byte("declared bytes"))}
	}
	record := map[string]any{
		"attestations": []any{
			map[string]any{"theme": "emergency", "source_document_ref": ref()},
		},
		"product_family": map[string]any{
			"shared_attestations": []any{
				map[string]any{"theme": "material_health", "source_document_ref": ref()},
			},
		},
	}

	off := findings.NewReport()
	VerifyFileReferences(recordDir, record, VerifyOptions{}, off)
	if len(off.Findings) != 0 {
		t.Fatalf("evidence sites must be silent without the flag, got %v", off.Findings)
	}

	on := findings.NewReport()
	VerifyFileReferences(recordDir, record, VerifyOptions{Evidence: true}, on)
	if len(on.Findings) != 2 {
		t.Fatalf("expected 2 findings under Evidence: true, got %v", on.Findings)
	}
	wantPaths := map[string]bool{
		"/attestations/0/source_document_ref":                       false,
		"/product_family/shared_attestations/0/source_document_ref": false,
	}
	for _, f := range on.Findings {
		if f.Level != findings.LevelError || f.Code != findings.CodeSourceFileHashMismatch {
			t.Errorf("expected a hash-mismatch ERROR, got %s %s at %s", f.Level, f.Code, f.Path)
		}
		if _, ok := wantPaths[f.Path]; !ok {
			t.Errorf("unexpected path %q", f.Path)
			continue
		}
		wantPaths[f.Path] = true
	}
	for p, seen := range wantPaths {
		if !seen {
			t.Errorf("no finding reported at %q", p)
		}
	}
}

// TestVerifyFileReferencesUsesRawArrayIndices confirms pointers carry the raw
// array index, so an entry skipped for shape does not shift the pointers of
// the entries after it. This matches the ledger's own pointer convention.
func TestVerifyFileReferencesUsesRawArrayIndices(t *testing.T) {
	recordDir := t.TempDir()
	ref := map[string]any{"filename": "missing.pdf", "sha256": strings.Repeat("a", 64)}
	record := map[string]any{
		"source_files": []any{
			"skipped, not a map",
			map[string]any{"file_type": "datasheet_pdf", "reference": ref},
		},
		"attestations": []any{
			"skipped, not a map",
			map[string]any{"theme": "emergency", "source_document_ref": ref},
		},
		"product_family": map[string]any{
			"shared_attestations": []any{
				"skipped, not a map",
				map[string]any{"theme": "material_health", "source_document_ref": ref},
			},
		},
	}
	report := findings.NewReport()
	VerifyFileReferences(recordDir, record, VerifyOptions{Evidence: true}, report)

	want := []string{
		"/attestations/1/source_document_ref",
		"/product_family/shared_attestations/1/source_document_ref",
		"/source_files/1/reference",
	}
	got := make([]string, 0, len(report.Findings))
	for _, f := range report.Findings {
		got = append(got, f.Path)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d findings, got %v", len(want), report.Findings)
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
			}
		}
		if !found {
			t.Errorf("no finding at %q; got %v", w, got)
		}
	}
}

// TestVerifyFileReferencesDualWrite covers the shape the shipped guidance
// produces: one file referenced from two sites. Each site is judged against
// its own declared hash and reports at its own pointer, so a consumer can see
// the verification status of every claim the record makes.
func TestVerifyFileReferencesDualWrite(t *testing.T) {
	content := []byte("datasheet bytes")
	sum := sha256Hex(content)
	build := func(cutsheetSum string) map[string]any {
		return map[string]any{
			"source_files": []any{
				map[string]any{"file_type": "datasheet_pdf", "reference": map[string]any{
					"filename": "specs.pdf", "sha256": sum,
				}},
			},
			"product_family": map[string]any{
				"cutsheet": map[string]any{"filename": "specs.pdf", "sha256": cutsheetSum},
			},
		}
	}

	t.Run("absent_reports_once_per_site", func(t *testing.T) {
		report := findings.NewReport()
		VerifyFileReferences(t.TempDir(), build(sum), VerifyOptions{}, report)
		if len(report.Findings) != 2 {
			t.Fatalf("expected one INFO per site, got %v", report.Findings)
		}
		for _, f := range report.Findings {
			if f.Level != findings.LevelInfo || f.Code != findings.CodeSourceFileNotFound {
				t.Errorf("expected not-found INFO, got %s %s at %s", f.Level, f.Code, f.Path)
			}
		}
	})

	t.Run("present_and_matching_is_silent_at_both", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "specs.pdf"), content)
		report := findings.NewReport()
		VerifyFileReferences(dir, build(sum), VerifyOptions{}, report)
		if len(report.Findings) != 0 {
			t.Fatalf("expected no findings, got %v", report.Findings)
		}
	})

	t.Run("each_site_is_judged_against_its_own_declared_hash", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "specs.pdf"), content)
		report := findings.NewReport()
		VerifyFileReferences(dir, build(sha256Hex([]byte("stale bytes"))), VerifyOptions{}, report)
		one(t, report.Findings, findings.LevelError, findings.CodeSourceFileHashMismatch,
			"/product_family/cutsheet", "mismatch")
	})
}

// TestRefPolicyEnabled pins the gate itself, including the zero value: a site
// whose author did not choose a policy must never inherit one at runtime. The
// drift guard fails the build before such a site can ship.
func TestRefPolicyEnabled(t *testing.T) {
	cases := []struct {
		policy refPolicy
		opts   VerifyOptions
		want   bool
	}{
		{policyDefault, VerifyOptions{}, true},
		{policyDefault, VerifyOptions{Evidence: true}, true},
		{policyEvidence, VerifyOptions{}, false},
		{policyEvidence, VerifyOptions{Evidence: true}, true},
		{policyUnset, VerifyOptions{}, false},
		{policyUnset, VerifyOptions{Evidence: true}, false},
	}
	for _, c := range cases {
		if got := c.policy.enabled(c.opts); got != c.want {
			t.Errorf("policy %d with Evidence=%v: enabled = %v, want %v", c.policy, c.opts.Evidence, got, c.want)
		}
	}
}

// one asserts the report holds exactly one finding, with the given level,
// code, pointer, and message fragment.
func one(t *testing.T, got []findings.Finding, level findings.Level, code findings.Code, pointer, fragment string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 finding, got %d (%v)", len(got), got)
	}
	f := got[0]
	if f.Level != level {
		t.Errorf("level = %s, want %s", f.Level, level)
	}
	if f.Code != code {
		t.Errorf("code = %q, want %q", f.Code, code)
	}
	if f.Path != pointer {
		t.Errorf("path = %q, want %q", f.Path, pointer)
	}
	if !strings.Contains(f.Message, fragment) {
		t.Errorf("message %q does not contain %q", f.Message, fragment)
	}
}

// TestVerifyFileReferencesFlagsPlaceholderHash pins the draft-placeholder
// warning: a reference declaring the 64-zero placeholder is flagged wherever it
// appears, whether or not the file is reachable, and the flag is a WARNING, so
// it never changes the exit code. The walk continues past it, so the existing
// outcomes are preserved: an absent file still reports INFO, and a present file
// that does not match still fails with an ERROR.
func TestVerifyFileReferencesFlagsPlaceholderHash(t *testing.T) {
	const placeholder = "0000000000000000000000000000000000000000000000000000000000000000"
	record := func(filename string) map[string]any {
		return map[string]any{"product_family": map[string]any{
			"cutsheet": map[string]any{"filename": filename, "sha256": placeholder},
		}}
	}

	t.Run("file absent", func(t *testing.T) {
		report := findings.NewReport()
		VerifyFileReferences(t.TempDir(), record("not-on-disk.pdf"), VerifyOptions{}, report)
		if report.HasErrors() {
			t.Errorf("a placeholder hash must not fail validation: %v", report.Findings)
		}
		wantFinding(t, report.Findings, findings.LevelWarning, findings.CodeSourceFilePlaceholderHash,
			"/product_family/cutsheet", "64-zero placeholder")
		wantFinding(t, report.Findings, findings.LevelInfo, findings.CodeSourceFileNotFound,
			"/product_family/cutsheet", "")
		if len(report.Findings) != 2 {
			t.Errorf("expected exactly the warning and the not-found info, got %d: %v", len(report.Findings), report.Findings)
		}
	})

	t.Run("file present", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "real.pdf"), []byte("real bytes"), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		report := findings.NewReport()
		VerifyFileReferences(dir, record("real.pdf"), VerifyOptions{}, report)
		// Strictness is preserved: a real file never hashes to sixty-four
		// zeros, so the mismatch is still an ERROR.
		if !report.HasErrors() {
			t.Errorf("a present file whose hash does not match must still be an ERROR: %v", report.Findings)
		}
		wantFinding(t, report.Findings, findings.LevelWarning, findings.CodeSourceFilePlaceholderHash,
			"/product_family/cutsheet", "64-zero placeholder")
		wantFinding(t, report.Findings, findings.LevelError, findings.CodeSourceFileHashMismatch,
			"/product_family/cutsheet", "")
		if len(report.Findings) != 2 {
			t.Errorf("expected exactly the warning and the mismatch error, got %d: %v", len(report.Findings), report.Findings)
		}
	})
}

// wantFinding asserts the report carries a finding with the given level, code,
// and pointer, and a message containing fragment. Unlike one(), it tolerates
// other findings alongside it, which the placeholder legs need: each expects
// the new warning plus the pre-existing outcome.
func wantFinding(t *testing.T, got []findings.Finding, level findings.Level, code findings.Code, pointer, fragment string) {
	t.Helper()
	for _, f := range got {
		if f.Level == level && f.Code == code && f.Path == pointer && strings.Contains(f.Message, fragment) {
			return
		}
	}
	t.Errorf("no %s %s finding at %q containing %q; got %v", level, code, pointer, fragment, got)
}
