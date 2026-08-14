package sheet

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The lexical checks in hashFile cannot see a symlink, so containment has to
// resolve both sides before the open. Without that, a bundle file replaced by
// a link to an out-of-root file would be hashed and its digest stamped into
// the emitted record and its dual-written cutsheet.

func writeSheetFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func skipWithoutSymlinks(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs elevated privileges on Windows")
	}
}

func TestHashFileFollowsSymlinkInsideAssetsRoot(t *testing.T) {
	skipWithoutSymlinks(t)
	root := t.TempDir()
	content := []byte("datasheet bytes")
	writeSheetFile(t, filepath.Join(root, "real.pdf"), content)
	if err := os.Symlink(filepath.Join(root, "real.pdf"), filepath.Join(root, "alias.pdf")); err != nil {
		t.Skipf("symlink not supported here: %v", err)
	}

	h := &fileHasher{assetsRoot: root}
	viaLink, err := h.hashFile("alias.pdf")
	if err != nil {
		t.Fatalf("a symlink inside the assets root must still hash: %v", err)
	}
	viaReal, err := h.hashFile("real.pdf")
	if err != nil {
		t.Fatalf("hash real.pdf: %v", err)
	}
	if viaLink != viaReal {
		t.Errorf("hash through the link = %s, want %s", viaLink, viaReal)
	}
	if h.sentinelStamped {
		t.Error("no sentinel should be stamped for a resolvable file")
	}
}

func TestHashFileRejectsSymlinkOutsideAssetsRoot(t *testing.T) {
	skipWithoutSymlinks(t)
	outer := t.TempDir()
	root := filepath.Join(outer, "assets")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeSheetFile(t, filepath.Join(outer, "secret.pdf"), []byte("out of root"))
	if err := os.Symlink(filepath.Join(outer, "secret.pdf"), filepath.Join(root, "looks-innocent.pdf")); err != nil {
		t.Skipf("symlink not supported here: %v", err)
	}

	// The escape is unconditional: allowMissing must not turn it into a
	// sentinel, because the file is present, just not where it may be read.
	for _, allowMissing := range []bool{false, true} {
		h := &fileHasher{assetsRoot: root, allowMissing: allowMissing}
		sum, err := h.hashFile("looks-innocent.pdf")
		if err == nil {
			t.Fatalf("allowMissing=%v: expected an error, got hash %s", allowMissing, sum)
		}
		if !strings.Contains(err.Error(), "outside the assets root") {
			t.Errorf("allowMissing=%v: error %q does not say the reference resolves outside the assets root", allowMissing, err)
		}
		if h.sentinelStamped {
			t.Errorf("allowMissing=%v: an escape must never stamp the sentinel", allowMissing)
		}
	}
}

// A dangling symlink is indistinguishable from an absent file for authoring
// purposes, so it keeps the missing-file behavior and the draft workflow is
// unchanged.
func TestHashFileDanglingSymlinkBehavesAsMissing(t *testing.T) {
	skipWithoutSymlinks(t)
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "never-created.pdf"), filepath.Join(root, "dangling.pdf")); err != nil {
		t.Skipf("symlink not supported here: %v", err)
	}

	strict := &fileHasher{assetsRoot: root}
	if _, err := strict.hashFile("dangling.pdf"); err == nil {
		t.Error("expected an error for a dangling symlink without --allow-missing-files")
	}

	lenient := &fileHasher{assetsRoot: root, allowMissing: true}
	sum, err := lenient.hashFile("dangling.pdf")
	if err != nil {
		t.Fatalf("a dangling symlink under allowMissing must stamp the sentinel: %v", err)
	}
	if sum != zeroSHA256 {
		t.Errorf("hash = %s, want the zero sentinel", sum)
	}
	if !lenient.sentinelStamped {
		t.Error("sentinelStamped must be set so the record is marked a draft")
	}
	if len(lenient.warnings) != 1 {
		t.Errorf("warnings = %v, want exactly one", lenient.warnings)
	}
}

// The assets root itself is commonly reached through a symlink (a temporary
// directory on macOS is), so the root has to be resolved too or every
// reference would look like an escape.
func TestHashFileAssetsRootBehindSymlinkStillHashes(t *testing.T) {
	skipWithoutSymlinks(t)
	outer := t.TempDir()
	realRoot := filepath.Join(outer, "real-assets")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeSheetFile(t, filepath.Join(realRoot, "specs.pdf"), []byte("datasheet bytes"))
	linkedRoot := filepath.Join(outer, "assets")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Skipf("symlink not supported here: %v", err)
	}

	h := &fileHasher{assetsRoot: linkedRoot}
	sum, err := h.hashFile("specs.pdf")
	if err != nil {
		t.Fatalf("a file under a symlinked assets root must hash: %v", err)
	}
	if len(sum) != 64 {
		t.Errorf("sha256 hex length = %d, want 64", len(sum))
	}
}

// TestHashFileCachesDigests pins the per-run digest cache: a hit answers from
// the map without re-reading the file, and a miss is never cached, so the
// per-record sentinel and warning semantics under allowMissing are unchanged.
//
// The cache is keyed on the post-EvalSymlinks path when the file exists and on
// the unresolved path when it does not, so a delete-between-calls probe would
// miss on any host whose temp directory sits behind a symlink (macOS). The
// probe here overwrites the content instead: the original digest coming back
// proves the cache answered.
func TestHashFileCachesDigests(t *testing.T) {
	root := t.TempDir()
	writeSheetFile(t, filepath.Join(root, "cutsheet.pdf"), []byte("first bytes"))

	digests := map[string]string{}
	first := &fileHasher{assetsRoot: root, digests: digests}
	want, err := first.hashFile("cutsheet.pdf")
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}

	writeSheetFile(t, filepath.Join(root, "cutsheet.pdf"), []byte("different bytes entirely"))

	// A second record's hasher shares the run's cache, so it must answer from
	// the map rather than re-reading the changed file.
	second := &fileHasher{assetsRoot: root, digests: digests}
	cached, err := second.hashFile("cutsheet.pdf")
	if err != nil {
		t.Fatalf("hashFile (cached): %v", err)
	}
	if cached != want {
		t.Errorf("cached digest = %s, want the first run's %s (the cache did not answer)", cached, want)
	}

	// A hasher with a fresh cache still reads the file, so the miss path is
	// intact rather than globally short-circuited.
	fresh := &fileHasher{assetsRoot: root, digests: map[string]string{}}
	reread, err := fresh.hashFile("cutsheet.pdf")
	if err != nil {
		t.Fatalf("hashFile (fresh cache): %v", err)
	}
	if reread == want {
		t.Errorf("a fresh cache must re-hash the changed file, got the stale digest %s", reread)
	}

	// Misses are never cached: the second record naming the same missing file
	// re-runs the sentinel and warning branch, so per-record DRAFT state stays
	// per-record.
	shared := map[string]string{}
	for i, h := range []*fileHasher{
		{assetsRoot: root, allowMissing: true, digests: shared},
		{assetsRoot: root, allowMissing: true, digests: shared},
	} {
		sum, err := h.hashFile("not-in-the-bundle.pdf")
		if err != nil {
			t.Fatalf("hasher %d: hashFile: %v", i, err)
		}
		if sum != zeroSHA256 {
			t.Errorf("hasher %d: digest = %s, want the zero sentinel", i, sum)
		}
		if !h.sentinelStamped {
			t.Errorf("hasher %d: sentinelStamped is false; the miss path must run per record", i)
		}
		if len(h.warnings) != 1 {
			t.Errorf("hasher %d: got %d warnings, want exactly 1", i, len(h.warnings))
		}
	}
	if len(shared) != 0 {
		t.Errorf("a missing file must never be cached, got %v", shared)
	}
}
