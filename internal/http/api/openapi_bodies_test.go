package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
)

// memPrefs keeps no preferences; it is there so PUT /me/preferences reaches
// its body instead of refusing for want of a store.
type memPrefs struct{}

func (memPrefs) GetPrefs(context.Context, string, string) (identity.Preferences, bool, error) {
	return identity.Preferences{}, false, nil
}

func (memPrefs) PutPrefs(context.Context, string, string, identity.Preferences, time.Time) error {
	return nil
}

// probeBodies sends every documented write an empty object and returns, per
// route pattern, the type its handler read the body into.
func probeBodies(t *testing.T) map[string]reflect.Type {
	t.Helper()
	mux := http.NewServeMux()
	fullAPI(t, mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	got := map[string]reflect.Type{}
	decodedInto = func(r *http.Request, v any) { got[r.Pattern] = reflect.TypeOf(v).Elem() }
	t.Cleanup(func() { decodedInto = nil })

	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(openAPISpec, &spec); err != nil {
		t.Fatal(err)
	}
	var ops []string
	for path, methods := range spec.Paths {
		for m := range methods {
			if m != "get" {
				ops = append(ops, strings.ToUpper(m)+" "+path)
			}
		}
	}
	// Deletes last, so a probe never removes what a later one reaches for.
	sort.Slice(ops, func(i, j int) bool {
		di, dj := strings.HasPrefix(ops[i], "DELETE"), strings.HasPrefix(ops[j], "DELETE")
		if di != dj {
			return dj
		}
		return ops[i] < ops[j]
	})
	fill := strings.NewReplacer("{tag}", "lt-1", "{name}", "pilot", "{id}", "probe")
	for _, op := range ops {
		method, path, _ := strings.Cut(op, " ")
		req, _ := http.NewRequest(method, srv.URL+fill.Replace(path), strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	return got
}

// The spec said what every write answers and nothing about what it takes:
// thirty-seven operations, not one requestBody (#27). A client had to read
// the Go source to learn that a device is enrolled with "tag" and
// "hardware", and since the server refuses unknown fields, a guess was a
// 400. Every body a handler reads is now a schema in the spec, and this
// test holds the two together: each field the handler decodes is
// documented with its type and meaning, each documented field is one the
// handler decodes, and an operation that reads no body documents none.
func TestDocumentedRequestBodiesMatchTheServer(t *testing.T) {
	read := probeBodies(t)
	if sites := decodeSites(t); len(read) != sites {
		t.Fatalf("the probe reached %d of the %d places a handler decodes a body; "+
			"a handler that refuses an empty object before decoding hides its body from this test", len(read), sites)
	}

	var spec struct {
		Paths map[string]map[string]struct {
			RequestBody *struct {
				Content map[string]struct {
					Schema bodySchema `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]bodySchema `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openAPISpec, &spec); err != nil {
		t.Fatal(err)
	}
	c := bodyCheck{t: t, schemas: spec.Components.Schemas}
	for path, ops := range spec.Paths {
		for method, op := range ops {
			where := strings.ToUpper(method) + " " + path
			ty, decodes := read[where]
			switch {
			case op.RequestBody == nil && decodes:
				t.Errorf("%s reads a %v body the spec does not document", where, ty)
			case op.RequestBody != nil && !decodes:
				t.Errorf("%s documents a request body its handler never reads", where)
			case op.RequestBody != nil:
				body, ok := op.RequestBody.Content["application/json"]
				if !ok {
					t.Errorf("%s: the request body is not application/json", where)
					continue
				}
				c.compare(where, ty, body.Schema)
			}
		}
	}
}

// decodeSites counts the places this package decodes a request body, so a
// handler the probe never reached cannot pass for one without a body.
func decodeSites(t *testing.T) int {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		n += strings.Count(string(b), "decode(r, &")
	}
	return n
}

type bodySchema struct {
	Ref                  string                `json:"$ref"`
	Type                 string                `json:"type"`
	Description          string                `json:"description"`
	Items                *bodySchema           `json:"items"`
	Required             []string              `json:"required"`
	Properties           map[string]bodySchema `json:"properties"`
	AdditionalProperties json.RawMessage       `json:"additionalProperties"`
}

type bodyCheck struct {
	t       *testing.T
	schemas map[string]bodySchema
}

// compare holds one Go type to the schema documenting it.
func (c bodyCheck) compare(where string, ty reflect.Type, s bodySchema) {
	c.t.Helper()
	for ty.Kind() == reflect.Pointer {
		ty = ty.Elem()
	}
	if s.Ref != "" {
		name := strings.TrimPrefix(s.Ref, "#/components/schemas/")
		ref, ok := c.schemas[name]
		if !ok {
			c.t.Errorf("%s: %s resolves to nothing", where, s.Ref)
			return
		}
		s = ref
	}
	want := map[reflect.Kind]string{reflect.String: "string", reflect.Bool: "boolean",
		reflect.Int: "integer", reflect.Int64: "integer", reflect.Float64: "number",
		reflect.Slice: "array", reflect.Map: "object", reflect.Struct: "object"}[ty.Kind()]
	if ty.Kind() == reflect.Interface {
		return // any JSON value; the schema may say anything
	}
	if want == "" {
		c.t.Errorf("%s: no rule for a Go %v; teach the test", where, ty)
		return
	}
	if s.Type != want {
		c.t.Errorf("%s is a %v, documented as %q (want %q)", where, ty, s.Type, want)
		return
	}
	switch ty.Kind() {
	case reflect.Slice:
		if s.Items == nil {
			c.t.Errorf("%s: an array without items", where)
			return
		}
		c.compare(where+"[]", ty.Elem(), *s.Items)
	case reflect.Map:
		var extra bodySchema
		if len(s.AdditionalProperties) == 0 || json.Unmarshal(s.AdditionalProperties, &extra) != nil {
			c.t.Errorf("%s: a map without an additionalProperties schema", where)
			return
		}
		c.compare(where+"{}", ty.Elem(), extra)
	case reflect.Struct:
		fields := jsonFields(ty)
		for name, f := range fields {
			p, ok := s.Properties[name]
			if !ok {
				c.t.Errorf("%s: the server reads %q, the spec does not document it", where, name)
				continue
			}
			if p.Ref == "" && strings.TrimSpace(p.Description) == "" {
				c.t.Errorf("%s.%s does not say what it means", where, name)
			}
			c.compare(where+"."+name, f, p)
		}
		for name := range s.Properties {
			if _, ok := fields[name]; !ok {
				c.t.Errorf("%s: the spec documents %q, which the server refuses as unknown", where, name)
			}
		}
		for _, r := range s.Required {
			if _, ok := fields[r]; !ok {
				c.t.Errorf("%s: requires %q, which is not a field", where, r)
			}
		}
	}
}

// jsonFields is what encoding/json reads from a struct: name to type.
func jsonFields(ty reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := range ty.NumField() {
		f := ty.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			for n, t := range jsonFields(f.Type) {
				out[n] = t
			}
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = f.Type
	}
	return out
}
