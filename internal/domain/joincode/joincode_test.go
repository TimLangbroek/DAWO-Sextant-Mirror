package joincode

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestNewUsesTheWholeAlphabetAndOnlyIt(t *testing.T) {
	// 40 zero bits, then 40 one bits: the first and the last character.
	for in, want := range map[string]string{
		"\x00\x00\x00\x00\x00": "00000000",
		"\xff\xff\xff\xff\xff": "ZZZZZZZZ",
		"\x84\x21\x08\x42\x10": "GGGGGGGG", // 10000 x8 = 16 = G
	} {
		got, err := New(strings.NewReader(in))
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("New(% x) = %s, want %s", in, got, want)
		}
	}
	seen := map[rune]bool{}
	for range 400 {
		c, err := New(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if n, err := Normalize(c); err != nil || n != c {
			t.Fatalf("a drawn code %q does not survive Normalize: %q %v", c, n, err)
		}
		for _, r := range c {
			seen[r] = true
		}
	}
	if len(seen) != 32 {
		t.Errorf("400 codes used %d of the 32 characters", len(seen))
	}
}

func TestNewFailsOnAShortRead(t *testing.T) {
	if _, err := New(strings.NewReader("abc")); err == nil {
		t.Fatal("a code was made from 24 bits")
	}
}

func TestNormalizeReadsWhatAPersonTypes(t *testing.T) {
	for typed, want := range map[string]string{
		"K7QM-4XWD":   "K7QM4XWD",
		"k7qm 4xwd":   "K7QM4XWD",
		" k7qm4xwd ":  "K7QM4XWD",
		"OIL0-1234":   "01101234",
		"oil0-1234":   "01101234",
		"K7QM--4XWD ": "K7QM4XWD",
	} {
		got, err := Normalize(typed)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", typed, got, err, want)
		}
	}
	for _, bad := range []string{"", "K7QM4XW", "K7QM4XWDD", "K7QM-4XWU", "K7QM_4XWD", "K7QM4XWé"} {
		if got, err := Normalize(bad); err == nil {
			t.Errorf("Normalize(%q) = %q, want an error", bad, got)
		}
	}
}

func TestFormat(t *testing.T) {
	if got := Format("K7QM4XWD"); got != "K7QM-4XWD" {
		t.Fatalf("Format = %s", got)
	}
	if got := Format("short"); got != "short" {
		t.Fatalf("Format of a non-code = %s", got)
	}
}

func TestHashNeedsTheKey(t *testing.T) {
	k1, k2 := Key([]byte("session key one")), Key([]byte("session key two"))
	if bytes.Equal(k1, []byte("session key one")) || len(k1) != 32 {
		t.Fatal("Key returned its input")
	}
	h := Hash(k1, "K7QM4XWD")
	// Pinned: a change to the derivation closes every open code at once.
	if got := hex.EncodeToString(h); got != "214230a36cba7d974faa199ceca9d9dff0da7b82de015968832024c3854e3b24" {
		t.Fatalf("the hash of a known code moved: %s", got)
	}
	if !bytes.Equal(h, Hash(k1, "K7QM4XWD")) {
		t.Fatal("the same code hashed twice differs")
	}
	if bytes.Equal(h, Hash(k2, "K7QM4XWD")) {
		t.Fatal("two keys give the same hash: the key does nothing")
	}
	if bytes.Equal(h, Hash(k1, "K7QM4XWE")) {
		t.Fatal("two codes give the same hash")
	}
}

func TestOpenFollowsTheClockAndTheUses(t *testing.T) {
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	c := Code{Uses: 1, Expires: now.Add(time.Minute)}
	if !c.Open(now) {
		t.Fatal("an unused code inside its window is closed")
	}
	if c.Open(now.Add(time.Minute)) {
		t.Fatal("a code is open at its expiry")
	}
	c.Uses = 0
	if c.Open(now) {
		t.Fatal("a spent code is open")
	}
}

func TestTagFromSerial(t *testing.T) {
	for serial, want := range map[string]string{
		"PF3ABCDE":                 "laptop-3abcde",
		"pf-3a bc/de":              "laptop-3abcde",
		"R9":                       "laptop-r9",
		"":                         "",
		"  ":                       "",
		"Default string":           "",
		"To Be Filled By O.E.M.":   "",
		"System Serial Number":     "",
		"---":                      "",
		"VMware-56 4d 9a 1b 2c 3d": "laptop-1b2c3d",
	} {
		if got := TagFromSerial(serial); got != want {
			t.Errorf("TagFromSerial(%q) = %q, want %q", serial, got, want)
		}
		if Serial(serial) != (want != "") {
			t.Errorf("Serial(%q) = %v", serial, Serial(serial))
		}
	}
}
