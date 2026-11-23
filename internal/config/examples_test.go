package config

import (
	"path/filepath"
	"testing"
)

// The test presets and the example sandbox file must stay valid.
func TestTestPresetsAndExample(t *testing.T) {
	dir := "../../test/presets"
	all, err := ListPresets(dir)
	if err != nil || len(all) < 5 {
		t.Fatalf("test presets: %d, %v", len(all), err)
	}
	file := "../../examples/burrow.sandbox"
	keys, err := LoadSandboxFile(file)
	if err != nil {
		t.Fatal(err)
	}
	p, err := FindPreset(keys["PRESET"], dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSandbox(p, keys, filepath.Dir(file), Overrides{}); err != nil {
		t.Fatal(err)
	}
}
