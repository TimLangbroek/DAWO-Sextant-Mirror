package config

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every refusal comes at once, each with what to do about it, so an
// operator fixes them in one round instead of one restart at a time.
func TestAllRefusalsAtOnce(t *testing.T) {
	_, err := Load([]string{"--log-level", "loud", "--log-format", "xml", "--write", "--gate", "none"}, env(nil))
	if err == nil {
		t.Fatal("want a refusal")
	}
	msg := err.Error()
	for _, want := range []string{"will not start", "log level \"loud\"", "log format \"xml\"", "nothing checks them", "no fleet repository"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal lacks %q:\n%s", want, msg)
		}
	}
	if n := strings.Count(msg, "\n    fix: "); n != 4 {
		t.Errorf("%d fix lines for four problems:\n%s", n, msg)
	}
	var r *Refusal
	if !errors.As(err, &r) {
		t.Fatal("the refusals are not Refusal values")
	}
}

func TestRefusalPointsAtTheHandbook(t *testing.T) {
	r := &Refusal{Problem: "p", Fix: "f", Doc: "deploy.html#troubleshooting"}
	if got := r.Error(); got != "p\n    fix: f\n    see: https://docs.sextantfleet.com/operators/deploy.html#troubleshooting" {
		t.Fatalf("got %q", got)
	}
	if got := (&Refusal{Problem: "p", Fix: "f"}).Error(); strings.Contains(got, "see:") {
		t.Fatalf("a refusal without a page points somewhere: %q", got)
	}
}

// A link in a refusal is read by someone whose console will not start; a
// dead one there is worse than none. Every page named in config.go exists
// in the handbook, and every anchor is a heading on it.
func TestRefusalLinksExist(t *testing.T) {
	src, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatal(err)
	}
	links := regexp.MustCompile(`"([a-z-]+)\.html(#[a-z-]+)?"`).FindAllStringSubmatch(string(src), -1)
	if len(links) == 0 {
		t.Fatal("found no handbook links in config.go")
	}
	for _, l := range links {
		page := filepath.Join("..", "..", "..", "docs", "handbook", "src", "operators", l[1]+".md")
		b, err := os.ReadFile(page)
		if err != nil {
			t.Errorf("%s.html: %v", l[1], err)
			continue
		}
		if l[2] == "" {
			continue
		}
		anchor := strings.TrimPrefix(l[2], "#")
		found := false
		for _, line := range strings.Split(string(b), "\n") {
			if h, ok := strings.CutPrefix(line, "#"); ok {
				slug := strings.ToLower(strings.Trim(strings.TrimLeft(h, "#"), " "))
				slug = strings.ReplaceAll(slug, " ", "-")
				found = found || slug == anchor
			}
		}
		if !found {
			t.Errorf("%s.html has no heading for #%s", l[1], anchor)
		}
	}
}
