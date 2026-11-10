package api

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// updateReference rewrites the handbook's API reference from the spec:
//
//	go test ./internal/http/api -run TestTheAPIReferenceIsCurrent -update
var updateReference = flag.Bool("update", false, "rewrite docs/handbook/src/reference/api.md from openapi.json")

// referencePath is the handbook page, from this package's directory.
var referencePath = filepath.Join("..", "..", "..", "docs", "handbook", "src", "reference", "api.md")

// The machine contract is served as JSON, which a client reads and a person
// does not (#27). The handbook carries the same contract as a page, made
// from openapi.json by renderReference, and this test fails when the two
// differ, so the page cannot describe an API the server no longer has.
func TestTheAPIReferenceIsCurrent(t *testing.T) {
	got, err := renderReference(openAPISpec)
	if err != nil {
		t.Fatal(err)
	}
	if *updateReference {
		if err := os.WriteFile(referencePath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatalf("%v; write it with: go test ./internal/http/api -run TestTheAPIReferenceIsCurrent -update", err)
	}
	if string(want) != got {
		t.Fatal("docs/handbook/src/reference/api.md is not what openapi.json says; " +
			"regenerate it with: go test ./internal/http/api -run TestTheAPIReferenceIsCurrent -update")
	}
}

// The page names every operation the spec has, and the table of contents
// and the sections agree: a renderer that dropped one would still produce
// a page that matches itself.
func TestTheAPIReferenceNamesEveryOperation(t *testing.T) {
	page, err := renderReference(openAPISpec)
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(openAPISpec, &spec); err != nil {
		t.Fatal(err)
	}
	n := 0
	for path, ops := range spec.Paths {
		for method := range ops {
			n++
			heading := fmt.Sprintf("### `%s %s`", strings.ToUpper(method), path)
			if !strings.Contains(page, heading+"\n") {
				t.Errorf("the page has no section %s", heading)
			}
		}
	}
	if n == 0 {
		t.Fatal("the spec has no operations; refusing to pass on nothing")
	}
	// The page's own comment and code spans aside, a "<" is a tag to Markdown.
	_, body, _ := strings.Cut(page, "-->")
	body = regexp.MustCompile("`[^`]*`").ReplaceAllString(body, "``")
	if i := strings.Index(body, "<"); i >= 0 {
		t.Errorf("the page has a bare %q, which Markdown reads as an HTML tag: ...%s...",
			"<", body[max(0, i-30):min(len(body), i+30)])
	}
}

type refSchema struct {
	Ref         string               `json:"$ref"`
	Type        string               `json:"type"`
	Description string               `json:"description"`
	Enum        []string             `json:"enum"`
	Items       *refSchema           `json:"items"`
	Required    []string             `json:"required"`
	Properties  map[string]refSchema `json:"properties"`
	// AdditionalProperties is a schema, or true; a map's values.
	AdditionalProperties json.RawMessage `json:"additionalProperties"`
}

type refParam struct {
	Ref         string    `json:"$ref"`
	Name        string    `json:"name"`
	In          string    `json:"in"`
	Description string    `json:"description"`
	Schema      refSchema `json:"schema"`
}

type refOp struct {
	Summary     string     `json:"summary"`
	Description string     `json:"description"`
	Deprecated  bool       `json:"deprecated"`
	Parameters  []refParam `json:"parameters"`
	RequestBody *struct {
		Content map[string]struct {
			Schema refSchema `json:"schema"`
		} `json:"content"`
	} `json:"requestBody"`
	Responses map[string]struct {
		Description string `json:"description"`
		Content     map[string]struct {
			Schema refSchema `json:"schema"`
		} `json:"content"`
	} `json:"responses"`
}

// renderReference turns the spec into the handbook page. Deterministic:
// paths, methods, codes and fields are sorted, so the same spec always
// gives the same bytes.
func renderReference(raw []byte) (string, error) {
	var spec struct {
		Info struct {
			Title, Version, Description string
		} `json:"info"`
		Paths      map[string]map[string]refOp `json:"paths"`
		Components struct {
			Schemas    map[string]refSchema `json:"schemas"`
			Parameters map[string]refParam  `json:"parameters"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return "", err
	}
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w("# API reference\n\n")
	w("<!-- Made from internal/http/api/openapi.json; do not edit by hand. Regenerate with\n")
	w("     go test ./internal/http/api -run TestTheAPIReferenceIsCurrent -update -->\n\n")
	w("Every console serves this contract as OpenAPI %s at `GET /api/v1/openapi.json`, without\n", "3.0")
	w("a credential. This page is the same document for reading. Contract version: %s.\n\n", spec.Info.Version)
	w("%s\n\n", prose(strings.TrimSpace(spec.Info.Description)))

	paths := make([]string, 0, len(spec.Paths))
	for p := range spec.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	methods := []string{"get", "post", "put", "patch", "delete"}

	w("## Operations\n\n| Operation | What it does |\n|---|---|\n")
	for _, p := range paths {
		for _, m := range methods {
			if op, ok := spec.Paths[p][m]; ok {
				w("| `%s %s` | %s |\n", strings.ToUpper(m), p, cell(op.Summary))
			}
		}
	}
	w("\n")

	for _, p := range paths {
		for _, m := range methods {
			op, ok := spec.Paths[p][m]
			if !ok {
				continue
			}
			w("### `%s %s`\n\n", strings.ToUpper(m), p)
			if op.Summary != "" {
				w("%s.", prose(strings.TrimSuffix(op.Summary, ".")))
				if op.Deprecated {
					w(" **Deprecated.**")
				}
				w("\n\n")
			}
			if d := strings.TrimSpace(op.Description); d != "" {
				w("%s\n\n", prose(d))
			}
			if len(op.Parameters) > 0 {
				w("| Parameter | In | Type | Meaning |\n|---|---|---|---|\n")
				for _, prm := range op.Parameters {
					if prm.Ref != "" {
						prm = spec.Components.Parameters[strings.TrimPrefix(prm.Ref, "#/components/parameters/")]
					}
					w("| `%s` | %s | %s | %s |\n", prm.Name, prm.In, typeOf(prm.Schema), cell(prm.Description))
				}
				w("\n")
			}
			if op.RequestBody != nil {
				if ct, ok := op.RequestBody.Content["application/json"]; ok {
					w("Request body: %s.\n\n", typeOf(ct.Schema))
				}
			}
			codes := make([]string, 0, len(op.Responses))
			for c := range op.Responses {
				codes = append(codes, c)
			}
			sort.Strings(codes)
			w("| Answer | Body | Meaning |\n|---|---|---|\n")
			for _, c := range codes {
				r := op.Responses[c]
				body := ""
				if ct, ok := r.Content["application/json"]; ok {
					body = typeOf(ct.Schema)
				}
				w("| `%s` | %s | %s |\n", c, body, cell(r.Description))
			}
			w("\n")
		}
	}

	names := make([]string, 0, len(spec.Components.Schemas))
	for n := range spec.Components.Schemas {
		names = append(names, n)
	}
	sort.Strings(names)
	w("## Schemas\n\n")
	w("Required means two things. In an answer, the field is always there. In a request\n")
	w("body, the server needs it. The server refuses a body with a field this page does not name.\n\n")
	for _, n := range names {
		s := spec.Components.Schemas[n]
		w("### %s\n\n", n)
		if d := strings.TrimSpace(s.Description); d != "" {
			w("%s\n\n", prose(d))
		}
		if len(s.Properties) == 0 {
			w("Type: %s.\n\n", typeOf(s))
			continue
		}
		req := map[string]bool{}
		for _, r := range s.Required {
			req[r] = true
		}
		fields := make([]string, 0, len(s.Properties))
		for f := range s.Properties {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		w("| Field | Type | Required | Meaning |\n|---|---|---|---|\n")
		for _, f := range fields {
			always := ""
			if req[f] {
				always = "yes"
			}
			w("| `%s` | %s | %s | %s |\n", f, typeOf(s.Properties[f]), always, cell(s.Properties[f].Description))
		}
		w("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

// typeOf names a schema the way the page shows it: a link for a named
// schema, "array of" for a list, the fields for a small inline object.
func typeOf(s refSchema) string {
	switch {
	case s.Ref != "":
		name := strings.TrimPrefix(s.Ref, "#/components/schemas/")
		return fmt.Sprintf("[%s](#%s)", name, strings.ToLower(name))
	case s.Type == "array" && s.Items != nil:
		return "array of " + typeOf(*s.Items)
	case len(s.Enum) > 0:
		return "one of `" + strings.Join(s.Enum, "`, `") + "`"
	case s.Type == "object" && len(s.AdditionalProperties) > 0:
		var values refSchema
		if err := json.Unmarshal(s.AdditionalProperties, &values); err != nil {
			return "object"
		}
		return "object of " + typeOf(values) + " values"
	case s.Type == "object" && len(s.Properties) > 0:
		fields := make([]string, 0, len(s.Properties))
		for f := range s.Properties {
			fields = append(fields, "`"+f+"`")
		}
		sort.Strings(fields)
		return "object with " + strings.Join(fields, ", ")
	case s.Type != "":
		return s.Type
	}
	return "any"
}

// cell makes text safe inside a table cell: one line, no bare pipes.
func cell(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.ReplaceAll(prose(s), "|", "\\|")
}

// prose makes spec text safe as Markdown: "group:<name>" would otherwise
// be read as an HTML tag, and the tag swallows the rest of the table.
func prose(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
