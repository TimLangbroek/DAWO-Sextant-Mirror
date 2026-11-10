// Package joincode models the short code an administrator gives a laptop so
// it can join the fleet (design 0015).
//
// WHAT A CODE IS. Eight characters a person can read aloud and type on a
// laptop that has no browser session: Crockford base32, so there is no I, L,
// O or U, and the letters people confuse with digits read as those digits.
// Forty bits. The code is the approval: whoever made it chose the group and
// how many laptops may use it, so a laptop that presents it joins without a
// second screen to watch.
//
// WHAT IS KEPT. Never the code. A code is stored as an HMAC under a key the
// database does not hold, so reading the table does not give the codes back
// - forty bits behind a plain hash would fall to a laptop's GPU in minutes.
// Expiry is a timestamp and nothing writes "expired": whether a code is still
// open is a function of the clock, as with elevation requests, so a code
// cannot outlive its window because whatever was going to close it was not
// running.
package joincode

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Length is the number of characters in a code, without the dash.
const Length = 8

// TTL is how long a code stays open. Long enough to make a code in the
// afternoon and use it the next morning; short enough that a code written on
// a sticky note is worthless by the time anyone else reads it.
const TTL = 24 * time.Hour

// MaxUses caps a batch code. A code for a whole delivery of laptops is
// reasonable; a code that works for anyone, indefinitely, is a password.
const MaxUses = 100

// alphabet is Crockford's base32: digits, then letters without I, L, O, U.
const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Code is one stored code. The code itself is not here (see the package
// comment); Hash is what a presented code is matched against.
type Code struct {
	ID   string `json:"id"`
	Hash []byte `json:"-"`
	// Group is where a joining laptop lands.
	Group string `json:"group,omitempty"`
	// Tag binds the code to an existing device: Re-enrol on a device's page.
	// Using it gives that device a new credential instead of making one.
	Tag       string    `json:"tag,omitempty"`
	Uses      int       `json:"uses"`
	Created   time.Time `json:"created"`
	CreatedBy string    `json:"createdBy,omitempty"`
	Expires   time.Time `json:"expires"`
}

// Open reports whether the code can still be used at now.
func (c Code) Open(now time.Time) bool {
	return c.Uses > 0 && now.Before(c.Expires)
}

// New draws a code from r (crypto/rand.Reader in production).
func New(r io.Reader) (string, error) {
	var b [5]byte // 40 bits, eight groups of five
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", fmt.Errorf("draw a join code: %w", err)
	}
	v := uint64(b[0])<<32 | uint64(b[1])<<24 | uint64(b[2])<<16 | uint64(b[3])<<8 | uint64(b[4])
	out := make([]byte, Length)
	for i := Length - 1; i >= 0; i-- {
		out[i] = alphabet[v&31]
		v >>= 5
	}
	return string(out), nil
}

// ErrMalformed is a presented code that cannot be a code at all. It gets the
// same answer as a wrong one, so the endpoint says nothing about which codes
// exist; the distinction is only for the agent's own message.
var ErrMalformed = errors.New("a join code is eight letters and digits")

// Normalize turns what a person typed into the canonical code: upper case,
// dashes and spaces dropped, O read as 0 and I or L as 1.
func Normalize(typed string) (string, error) {
	var b strings.Builder
	for _, r := range strings.ToUpper(typed) {
		switch {
		case r == '-' || r == ' ':
			continue
		case r == 'O':
			r = '0'
		case r == 'I' || r == 'L':
			r = '1'
		}
		if !strings.ContainsRune(alphabet, r) {
			return "", ErrMalformed
		}
		b.WriteRune(r)
	}
	if b.Len() != Length {
		return "", ErrMalformed
	}
	return b.String(), nil
}

// Format shows a canonical code the way a person reads it: XXXX-XXXX.
func Format(code string) string {
	if len(code) != Length {
		return code
	}
	return code[:4] + "-" + code[4:]
}

// Key derives the key codes are hashed under from a secret the console
// already holds, so a code key never needs storing or rotating of its own.
// The label keeps it from being the same value as anything else derived
// from that secret.
func Key(secret []byte) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("sextant join codes v1"))
	return m.Sum(nil)
}

// Hash is what is stored for a canonical code, and what a presented code is
// looked up by.
func Hash(key []byte, code string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(code))
	return m.Sum(nil)
}

// TagFromSerial proposes a device tag from a serial number: "laptop-" and
// its last six letters and digits, lower case. Empty when the serial has
// none, or is one of the placeholders firmware fills in when nobody set it.
func TagFromSerial(serial string) string {
	switch strings.ToLower(strings.TrimSpace(serial)) {
	case "default string", "to be filled by o.e.m.", "system serial number",
		"not specified", "none", "0", "0123456789", "123456789":
		return ""
	}
	var keep []byte
	for _, r := range strings.ToLower(serial) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			keep = append(keep, byte(r))
		}
	}
	if len(keep) == 0 {
		return ""
	}
	if len(keep) > 6 {
		keep = keep[len(keep)-6:]
	}
	return "laptop-" + string(keep)
}

// Serial reports whether a serial number identifies a machine, so it can be
// used to tell two laptops apart. The firmware placeholders above do not.
func Serial(serial string) bool {
	return TagFromSerial(serial) != ""
}
