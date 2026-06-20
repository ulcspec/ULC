package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/grade"
)

// TestBuildConformanceMatchesGrade is the order-independent build-parity guard for
// the conformance projection: for every example, the token Build() stamps into
// index.conformance_level must equal grade.AchievedLevel(record).String(). It reads
// the freshly computed values (never the stored index), so it holds both before and
// after the re-stamp step and verifies builder.go's switch stays in lockstep with
// the grader's ladder (including the incomplete token).
func TestBuildConformanceMatchesGrade(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	examplesDir := filepath.Join(root, "examples")
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("read examples: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".ulc" {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			record, err := loadRecord(filepath.Join(examplesDir, name))
			if err != nil {
				t.Fatalf("load %s: %v", name, err)
			}
			built := Build(record)
			got, _ := built["conformance_level"].(string)
			want := grade.AchievedLevel(record).String()
			if got != want {
				t.Errorf("%s: Build() conformance_level = %q, grade.AchievedLevel = %q", name, got, want)
			}
		})
	}
}
