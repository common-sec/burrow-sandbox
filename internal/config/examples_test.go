package config

import (
	"path/filepath"
	"testing"
)

// The shipped examples must stay valid.
func TestExamples(t *testing.T) {
	envs, _ := filepath.Glob("../../examples/envs/*.env")
	presets, _ := filepath.Glob("../../examples/presets/*.preset")
	if len(envs) == 0 || len(presets) == 0 {
		t.Fatal("no examples found")
	}
	for _, f := range envs {
		if _, err := LoadEnv(f); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	for _, f := range presets {
		if _, err := LoadPreset(f); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}
