package sandbox

import (
	"context"
	"strings"
	"testing"

	"github.com/common-sec/burrow-sandbox/internal/podman"
)

func TestCheckEngine(t *testing.T) {
	for info, want := range map[string]string{
		"4.9.3 netavark":       "",
		"5.6.0 netavark":       "",
		"4.10.0 netavark":      "", // numeric, not string, comparison
		"5.0.0-dev netavark":   "",
		"4.8.3 netavark":       "too old",
		"3.4.4 cni":            "too old",
		"4.9.3 cni":            "netavark",
		"4.9.3":                "unexpected",
		"x.y netavark":         "too old",
		"4.9.3 netavark extra": "unexpected",
	} {
		err := checkEngine(info)
		if want == "" && err != nil || want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
			t.Errorf("checkEngine(%q) = %v, want %q", info, err, want)
		}
	}
}

func TestPreflightAsksPodmanInfo(t *testing.T) {
	rec := &podman.Recorder{Replies: map[string]string{"info": "4.9.3 netavark"}}
	if err := Preflight(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	if len(rec.Calls) != 1 || rec.Calls[0][0] != "info" {
		t.Fatalf("calls = %v", rec.Calls)
	}
}
