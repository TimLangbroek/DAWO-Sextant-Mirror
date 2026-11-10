package api

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A path such as /api/v1/devices/{tag} takes a value the spec has to name:
// without it a generated client has no argument for the tag, and a person
// reading the page does not learn what goes there (#27). Every placeholder
// in a path is a required path parameter with a type and a meaning, and no
// operation documents a path parameter its path does not have.
func TestEveryPathParameterIsDocumented(t *testing.T) {
	var spec struct {
		Paths      map[string]map[string]refOp `json:"paths"`
		Components struct {
			Parameters map[string]struct {
				refParam
				Required bool `json:"required"`
			} `json:"parameters"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openAPISpec, &spec); err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Ref      string `json:"$ref"`
				In       string `json:"in"`
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(openAPISpec, &raw); err != nil {
		t.Fatal(err)
	}
	placeholder := regexp.MustCompile(`\{(\w+)\}`)
	paths := make([]string, 0, len(raw.Paths))
	for p := range raw.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	checked := 0
	for _, path := range paths {
		want := map[string]bool{}
		for _, m := range placeholder.FindAllStringSubmatch(path, -1) {
			want[m[1]] = true
		}
		for method, op := range raw.Paths[path] {
			where := strings.ToUpper(method) + " " + path
			got := map[string]bool{}
			for _, prm := range op.Parameters {
				name, in, required := prm.Name, prm.In, prm.Required
				desc, typ := "", ""
				if prm.Ref != "" {
					c, ok := spec.Components.Parameters[strings.TrimPrefix(prm.Ref, "#/components/parameters/")]
					if !ok {
						t.Errorf("%s: %s resolves to nothing", where, prm.Ref)
						continue
					}
					name, in, required, desc, typ = c.Name, c.In, c.Required, c.Description, c.Schema.Type
				} else {
					inline := spec.Paths[path][method].Parameters
					for _, ip := range inline {
						if ip.Name == name && ip.In == in {
							desc, typ = ip.Description, ip.Schema.Type
						}
					}
				}
				if in != "path" {
					continue
				}
				checked++
				got[name] = true
				switch {
				case !want[name]:
					t.Errorf("%s documents path parameter %q, which the path does not have", where, name)
				case !required:
					t.Errorf("%s: path parameter %q must be required", where, name)
				case typ == "":
					t.Errorf("%s: path parameter %q has no type", where, name)
				case strings.TrimSpace(desc) == "":
					t.Errorf("%s: path parameter %q does not say what goes there", where, name)
				}
			}
			for name := range want {
				if !got[name] {
					t.Errorf("%s does not document its path parameter %q", where, name)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no path parameter was checked; refusing to pass on nothing")
	}
}
