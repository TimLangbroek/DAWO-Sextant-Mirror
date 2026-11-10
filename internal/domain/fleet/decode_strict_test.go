package fleet

import (
	"strings"
	"testing"
)

// A field this build does not know is refused, not dropped (#18). Dropping
// was the default of encoding/json, and it was worse than a lost read: the
// next console write encoded the document without the field, so a policy
// section spelled by a newer Sextant, or misspelled by hand, vanished from
// the repository in a commit that said something else.
func TestDecodeRefusesWhatItWouldDrop(t *testing.T) {
	for _, tc := range []struct{ name, doc, field string }{
		{"a section", `{"version": 3, "polices": {"p": {"settings": {"x": 1}}}}`, "polices"},
		{"a device field", `{"version": 3, "devices": {"lt-1": {"hardware": "x", "groupz": ["pilot"]}}}`, "groupz"},
		{"an assignment field", `{"version": 3, "policies": {"p": {"settings": {"x": 1}}},
			"assignments": [{"policy": "p", "target": "org", "scope": "group:pilot"}]}`, "scope"},
		{"a rule field", `{"version": 3, "filters": {"f": {"rules": [{"attr": "class", "op": "eq", "vaule": "laptop"}]}}}`, "vaule"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode([]byte(tc.doc))
			if err == nil {
				t.Fatalf("decoded a document carrying %q, which this build would drop", tc.field)
			}
			for _, want := range []string{`"` + tc.field + `"`, "does not know", "upgrade Sextant"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal %q does not say %s", err, want)
				}
			}
		})
	}
}

// What is free-form stays free-form: a setting's key is the overlay's
// vocabulary, not this build's, and an unknown one is the catalog's
// business, not the decoder's.
func TestDecodeKeepsFreeFormSettings(t *testing.T) {
	f, err := Decode([]byte(`{"version": 3, "org": {"settings": {"anything.at.all": {"nested": [1, 2]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Org.Settings["anything.at.all"]; !ok {
		t.Fatalf("settings = %v", f.Org.Settings)
	}
}

// One document, and nothing after it: the decoder reads a stream, and a
// second value behind the first is not something to ignore either.
func TestDecodeRefusesTrailingData(t *testing.T) {
	if _, err := Decode([]byte(`{"version": 3} {"version": 3}`)); err == nil {
		t.Fatal("decoded a document with a second one behind it")
	}
	if _, err := Decode([]byte("{\"version\": 3}\n\n")); err != nil {
		t.Fatalf("trailing whitespace refused: %v", err)
	}
}
