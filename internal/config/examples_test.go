package config

import (
	"path/filepath"
	"testing"

	"github.com/common-sec/burrow-sandbox/presets"
)

// The shipped presets and the example sandbox file must stay valid.
func TestBuiltinPresetsAndExample(t *testing.T) {
	list := ListPresets("", presets.FS)
	if len(list) == 0 {
		t.Fatal("no built-in presets")
	}
	for _, p := range list {
		if _, err := FindPreset(p.Name, "", presets.FS); err != nil {
			t.Errorf("%s: %v", p.Name, err)
		}
	}
	file := "../../examples/burrow.sandbox"
	keys, err := LoadSandboxFile(file)
	if err != nil {
		t.Fatal(err)
	}
	p, err := FindPreset(keys["PRESET"], "", presets.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSandbox(p, keys, filepath.Dir(file), Overrides{}); err != nil {
		t.Fatal(err)
	}
}
