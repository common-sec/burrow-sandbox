package consent

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type fakeTTY struct {
	answer string
	fail   bool
	asked  bytes.Buffer
}

func (f *fakeTTY) Open() (io.ReadWriteCloser, error) {
	if f.fail {
		return nil, errors.New("no tty")
	}
	return struct {
		io.Reader
		io.Writer
		io.Closer
	}{strings.NewReader(f.answer), &f.asked, io.NopCloser(nil)}, nil
}

var (
	fromPreset = Weakening{Name: Root, From: "RUN_AS in preset kali (/p/kali.preset)"}
	fromFlag   = Weakening{Name: Root, From: "--user", Explicit: true}
)

// U23: a weakening from a file is confirmed by a person, by a named flag, or refused. Each row
// is one way to get it through; only the person's answer or the flag may.
func TestCheck(t *testing.T) {
	for _, c := range []struct {
		name string
		p    Policy
		ws   []Weakening
		ok   bool
	}{
		{"nothing weakened", Policy{}, nil, true},
		{"--user 0:0 is explicit", Policy{CI: true}, []Weakening{fromFlag}, true},
		{"--accept root", Policy{Accepted: map[string]bool{Root: true}, CI: true}, []Weakening{fromPreset}, true},
		{"person answers y", Policy{Terminal: &fakeTTY{answer: "y\n"}}, []Weakening{fromPreset}, true},
		{"person answers yes", Policy{Terminal: &fakeTTY{answer: " YES \n"}}, []Weakening{fromPreset}, true},
		{"enter means no", Policy{Terminal: &fakeTTY{answer: "\n"}}, []Weakening{fromPreset}, false},
		{"anything else is no", Policy{Terminal: &fakeTTY{answer: "sure\n"}}, []Weakening{fromPreset}, false},
		{"no answer (EOF)", Policy{Terminal: &fakeTTY{}}, []Weakening{fromPreset}, false},
		{"no terminal (an agent, a script)", Policy{Terminal: &fakeTTY{fail: true}}, []Weakening{fromPreset}, false},
		{"no terminal at all", Policy{}, []Weakening{fromPreset}, false},
		// CI mode never asks, even when a terminal answers yes: CI=true is no way around it.
		{"CI mode with a yes on the terminal", Policy{CI: true, Terminal: &fakeTTY{answer: "y\n"}}, []Weakening{fromPreset}, false},
		{"accepted something else", Policy{Accepted: map[string]bool{"other": true}}, []Weakening{fromPreset}, false},
	} {
		err := c.p.Check(c.ws)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
		}
		if err != nil && (!errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "--accept root")) {
			t.Errorf("%s: the refusal must name the flag to add: %v", c.name, err)
		}
	}
}

// The question names the setting and where it came from.
func TestQuestionNamesTheSource(t *testing.T) {
	tty := &fakeTTY{answer: "n\n"}
	_ = Policy{Terminal: tty}.Check([]Weakening{fromPreset})
	if q := tty.asked.String(); !strings.Contains(q, "preset kali") || !strings.Contains(q, "[y/N]") {
		t.Fatalf("question = %q", q)
	}
}

// A crafted file name cannot put terminal escape sequences into the question.
func TestQuestionHasNoControlCharacters(t *testing.T) {
	tty := &fakeTTY{answer: "n\n"}
	w := Weakening{Name: Root, From: "RUN_AS in preset x (/p/\x1b[2K\rx.preset)"}
	_ = Policy{Terminal: tty}.Check([]Weakening{w})
	if strings.ContainsAny(tty.asked.String(), "\x1b\r") {
		t.Fatalf("question carries control characters: %q", tty.asked.String())
	}
}

func TestDryRunReportsWithoutAsking(t *testing.T) {
	var out bytes.Buffer
	tty := &fakeTTY{fail: true}
	if err := (Policy{DryRun: true, Report: &out, Terminal: tty}).Check([]Weakening{fromPreset}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "--accept root") {
		t.Fatalf("dry run report = %q", out.String())
	}
}

func TestParseAccept(t *testing.T) {
	for _, c := range []struct {
		in string
		ok bool
	}{
		{"root", true},
		{"", true},
		{" root , root ", true},
		{"ROOT", false},     // names are exact
		{"root,all", false}, // no wildcard that accepts future weakenings
		{"*", false},
		{"root\nother", false},
	} {
		_, err := ParseAccept(c.in)
		if (err == nil) != c.ok {
			t.Errorf("ParseAccept(%q): err=%v, want ok=%v", c.in, err, c.ok)
		}
	}
}

func TestCIFromEnv(t *testing.T) {
	for v, want := range map[string]bool{
		"true": true, "1": true, "TRUE": true, "yes": true,
		"": false, "false": false, "0": false, "no": false,
	} {
		if got := CIFromEnv(func(string) string { return v }); got != want {
			t.Errorf("CI=%q: %v, want %v", v, got, want)
		}
	}
}
