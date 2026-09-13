package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFinishedRecordCommandsRefuseRetiredName(t *testing.T) {
	retired := finishedRecordExtension + jsonSerializationSuffix
	commands := []struct {
		name string
		run  func([]string) int
	}{
		{"validate", runValidate},
		{"build-index", runBuildIndex},
		{"scope", runScope},
	}
	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			missing := filepath.Join(t.TempDir(), "record"+retired)
			stdout, stderr, code := captureOutErr(t, func() int {
				return command.run([]string{missing})
			})
			if code != 2 {
				t.Errorf("exit = %d, want 2", code)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, missing) || !strings.Contains(stderr, retired) {
				t.Errorf("stderr does not name path and retired suffix:\n%s", stderr)
			}
			if strings.Contains(stderr, "no such file") {
				t.Errorf("command read the refused path before rejecting its name:\n%s", stderr)
			}
		})
	}
}

func TestFinishedRecordCommandsRefuseRetiredNameCaseInsensitively(t *testing.T) {
	retired := finishedRecordExtension + jsonSerializationSuffix
	commands := []struct {
		name string
		run  func([]string) int
	}{
		{"validate", runValidate},
		{"build-index", runBuildIndex},
		{"scope", runScope},
	}
	mixedCase := strings.ToUpper(retired[:2]) + retired[2:]
	for _, suffix := range []string{strings.ToUpper(retired), mixedCase} {
		for _, command := range commands {
			t.Run(command.name+"/"+suffix, func(t *testing.T) {
				missing := filepath.Join(t.TempDir(), "record"+suffix)
				_, stderr, code := captureOutErr(t, func() int {
					return command.run([]string{missing})
				})
				if code != 2 {
					t.Errorf("exit = %d, want pre-read refusal exit 2; stderr:\n%s", code, stderr)
				}
				if !strings.Contains(stderr, missing) || !strings.Contains(stderr, retired) {
					t.Errorf("stderr does not name path and canonical retired suffix:\n%s", stderr)
				}
				if strings.Contains(stderr, "no such file") {
					t.Errorf("command read the refused path before rejecting its name:\n%s", stderr)
				}
			})
		}
	}
}

func TestFinishedRecordCommandsRefuseOnlyRetiredName(t *testing.T) {
	source := exampleRecord(t, vodeRecord)
	recordBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "record.working.json")
	if err := os.WriteFile(path, recordBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	commands := []struct {
		name string
		run  func([]string) int
		args []string
	}{
		{"validate", runValidate, []string{path}},
		{"build-index", runBuildIndex, []string{path, "--stdout"}},
		{"scope", runScope, []string{path}},
	}
	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			_, stderr, code := captureOutErr(t, func() int {
				return command.run(command.args)
			})
			if code != 0 {
				t.Errorf("exit = %d, want 0; stderr:\n%s", code, stderr)
			}
		})
	}
}
