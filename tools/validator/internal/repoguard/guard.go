// Package repoguard holds tracked-tree checks for active repository contracts.
package repoguard

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Line is one tracked source line presented to the matcher.
type Line struct {
	Path   string
	Number int
	Text   string
}

// Result is the tracked-tree scan and the lines that violate its contract.
type Result struct {
	Root         string
	TrackedFiles []string
	Matches      []Line
}

var legitimateRetiredNameExemptions = map[string]bool{
	// Release history truthfully records names shipped by earlier releases.
	"CHANGELOG.md": true,
	// This past-tense note explains the authoring templates removed earlier.
	"templates/README.md": true,
}

const maxTrackedFileBytes int64 = 8 << 20

func retiredRecordToken() string {
	return strings.Join([]string{".", "ulc", ".", "json"}, "")
}

var retiredNameNormalizer = strings.NewReplacer(
	"\\", "",
	"(", "",
	")", "",
	"[", "",
	"]", "",
	"?", "",
	":", "",
)

// FindRetiredRecordNames returns every non-exempt line whose normalized text
// carries the retired record token. Matches preserve the original line text.
func FindRetiredRecordNames(lines []Line, exemptions map[string]bool) []Line {
	needle := strings.ToLower(retiredRecordToken())
	var matches []Line
	for _, line := range lines {
		if exemptions[filepath.ToSlash(line.Path)] {
			continue
		}
		normalized := retiredNameNormalizer.Replace(line.Text)
		if strings.Contains(strings.ToLower(normalized), needle) {
			matches = append(matches, line)
		}
	}
	return matches
}

// FindRetiredRecordPaths returns tracked paths whose base name uses the retired
// finished-record suffix. Path matches use line number zero because the name is
// not part of the file content.
func FindRetiredRecordPaths(paths []string, exemptions map[string]bool) []Line {
	needle := strings.ToLower(retiredRecordToken())
	var matches []Line
	for _, path := range paths {
		path = filepath.ToSlash(path)
		if exemptions[path] || !strings.HasSuffix(strings.ToLower(filepath.Base(path)), needle) {
			continue
		}
		matches = append(matches, Line{Path: path, Text: "tracked filename uses the retired finished-record suffix"})
	}
	return matches
}

// ScanTrackedTree locates the repository containing start, enumerates its
// tracked files with git, and checks their original lines with the matcher.
func ScanTrackedTree(start string) (Result, error) {
	root, err := findRepositoryRoot(start)
	if err != nil {
		return Result{}, err
	}
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		return Result{}, fmt.Errorf("enumerate tracked files: %w", err)
	}
	parts := bytes.Split(out, []byte{0})
	tracked := make([]string, 0, len(parts))
	var lines []Line
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		rel := filepath.ToSlash(string(part))
		tracked = append(tracked, rel)
		path := filepath.Join(root, filepath.FromSlash(rel))
		data, err := readTrackedPath(path, rel)
		if err != nil {
			return Result{}, err
		}
		for i, text := range strings.Split(string(data), "\n") {
			lines = append(lines, Line{Path: rel, Number: i + 1, Text: text})
		}
	}
	if len(tracked) == 0 {
		return Result{}, fmt.Errorf("repository at %s has no tracked files", root)
	}
	return Result{
		Root:         root,
		TrackedFiles: tracked,
		Matches: append(
			FindRetiredRecordPaths(tracked, legitimateRetiredNameExemptions),
			FindRetiredRecordNames(lines, legitimateRetiredNameExemptions)...,
		),
	}, nil
}

func findRepositoryRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve start directory: %w", err)
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	for {
		info, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no repository root containing go.mod above %s", start)
		}
		dir = parent
	}
}

func readTrackedPath(path, rel string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect tracked file %s: %w", rel, err)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return nil, fmt.Errorf("read tracked symlink %s: %w", rel, err)
		}
		return []byte(target), nil
	case !info.Mode().IsRegular():
		return nil, fmt.Errorf("tracked path %s is not a regular file or symbolic link", rel)
	case info.Size() > maxTrackedFileBytes:
		return nil, fmt.Errorf("tracked file %s is %d bytes, above the %d-byte scan limit", rel, info.Size(), maxTrackedFileBytes)
	default:
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read tracked file %s: %w", rel, err)
		}
		return data, nil
	}
}
