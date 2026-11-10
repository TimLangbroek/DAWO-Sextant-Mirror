package fleet

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// migrate.go: forward migration of the fleet document across schema versions.
// A document older than this build is upgraded step by step to the current
// Version instead of being hard-rejected, so bumping the schema (v3 -> v4)
// never bricks an existing overlay. A document NEWER than this build is still
// rejected: an old binary must not silently misread a future document.

// migration upgrades a decoded fleet document one version forward, operating
// on the generic JSON map so it can add, rename or reshape fields the current
// Fleet struct no longer (or does not yet) know. Key N in the registry
// migrates a vN document to v(N+1).
type migration func(doc map[string]any) (map[string]any, error)

// migrations is the forward-migration registry. It is empty today: Version
// has been 3 since the schema was introduced, so no v1->v2 or v2->v3 step
// exists. The seam is shipped now so a future v3->v4 change adds one entry
// (`3: func(doc) {...}`) and existing documents upgrade on read.
var migrations = map[int]migration{}

// decode is Decode with an injectable registry, so tests exercise the
// migration path without a real future schema.
func decode(b []byte, migs map[int]migration) (*Fleet, error) {
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, fmt.Errorf("parse fleet document: %w", err)
	}
	switch {
	case probe.Version > Version:
		// A newer document: refuse rather than misread. The operator must
		// upgrade this build.
		return nil, fmt.Errorf("fleet document version %d is newer than this build supports (%d); upgrade Sextant", probe.Version, Version)
	case probe.Version < 1:
		return nil, fmt.Errorf("fleet document version %d: unsupported (expected 1..%d)", probe.Version, Version)
	case probe.Version < Version:
		var err error
		if b, err = runMigrations(b, probe.Version, migs); err != nil {
			return nil, err
		}
	}

	f, err := decodeStrict(b)
	if err != nil {
		return nil, err
	}
	if f.Version != Version {
		// A migration that forgot to stamp the new version, or a document that
		// slipped through: fail loudly rather than run on a half-migrated doc.
		return nil, fmt.Errorf("fleet document is version %d after migration, expected %d", f.Version, Version)
	}
	return f, nil
}

// decodeStrict refuses a field this build does not know instead of dropping
// it (#18). encoding/json drops by default, and here that was worse than a
// misread: the next console write encodes the document without the field,
// so a section a newer Sextant wrote, or a key misspelled by hand, left the
// repository in a commit about something else. Settings stay free-form;
// they are maps, and their keys are the overlay's, not this build's.
func decodeStrict(b []byte) (*Fleet, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var f Fleet
	if err := dec.Decode(&f); err != nil {
		if field, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
			return nil, fmt.Errorf("fleet document has a field %s that this build does not know; "+
				"upgrade Sextant if a newer one wrote it, or correct it if it is a typo "+
				"(reading on would drop it at the next write)", field)
		}
		return nil, fmt.Errorf("parse fleet document: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("parse fleet document: more than one JSON value")
	}
	return &f, nil
}

// runMigrations walks the registry from `from` up to Version, one step at a
// time, stamping the new version after each step.
func runMigrations(b []byte, from int, migs map[int]migration) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse fleet document: %w", err)
	}
	for v := from; v < Version; v++ {
		m, ok := migs[v]
		if !ok {
			return nil, fmt.Errorf("no migration from fleet version %d to %d", v, v+1)
		}
		next, err := m(doc)
		if err != nil {
			return nil, fmt.Errorf("migrate fleet v%d->v%d: %w", v, v+1, err)
		}
		next["version"] = float64(v + 1) // JSON numbers decode to float64
		doc = next
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("re-encode migrated fleet: %w", err)
	}
	return out, nil
}
