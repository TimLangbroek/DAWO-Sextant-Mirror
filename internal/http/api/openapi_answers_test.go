package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The spec said 200 for four creates that answer 201, called the answer
// of a credential, a tick and the preferences a bare status, and had no
// schema at all for fourteen answers. Nothing caught it: the contract
// tests read what six GETs answer and never made a write succeed. A
// generated client that trusts the spec fails on an answer it was not
// told about, and a person reading the reference learns nothing about
// what comes back.
//
// This walks every operation in the spec - writes with bodies the spec
// documents, in an order where each finds what it needs, then every read -
// and holds each answer to the spec: the status and the content type are
// documented, and the value the handler wrote has the documented shape,
// field by field, down to what sits behind a map of any. It fails on an
// operation it does not walk, so a new one cannot slip past.
func TestEveryOperationAnswersAsDocumented(t *testing.T) {
	mux := http.NewServeMux()
	fullAPI(t, mux)
	var last *answer
	answeredWith = func(status int, v any) { last = &answer{status, v} }
	t.Cleanup(func() { answeredWith = nil })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = nil
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	var spec struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]struct {
					Schema bodySchema `json:"schema"`
				} `json:"content"`
			} `json:"responses"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]bodySchema `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(openAPISpec, &spec); err != nil {
		t.Fatal(err)
	}
	check := answerCheck{t: t, schemas: spec.Components.Schemas}

	do := func(method, path, body string) (int, string, []byte) {
		t.Helper()
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		req, _ := http.NewRequest(method, srv.URL+path, rd)
		req.Header.Set("Authorization", "Bearer "+testToken)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		ct, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";")
		return resp.StatusCode, ct, b
	}

	walked := map[string]bool{}
	step := func(method, pattern, path, body string) []byte {
		t.Helper()
		op, ok := spec.Paths[pattern][strings.ToLower(method)]
		if !ok {
			t.Fatalf("%s %s is not in the spec", method, pattern)
		}
		where := method + " " + pattern
		walked[where] = true
		code, ct, got := do(method, path, body)
		if code < 200 || code > 299 {
			t.Fatalf("%s %s with %s: %d %s", method, path, body, code, got)
		}
		documented, ok := op.Responses[strconv.Itoa(code)]
		if !ok {
			t.Errorf("%s answers %d, which the spec does not document", where, code)
			return got
		}
		content, ok := documented.Content[ct]
		if !ok {
			t.Errorf("%s answers %s, which the spec does not document for %d", where, ct, code)
			return got
		}
		if ct != "application/json" {
			return got
		}
		if last == nil {
			t.Errorf("%s answered JSON without writeJSON; the test cannot see its value", where)
			return got
		}
		check.value(where, reflect.ValueOf(last.v), content.Schema)
		return got
	}

	step("PUT", "/api/v1/me/preferences", "/api/v1/me/preferences", `{"timezone":"Europe/Amsterdam","locale":"nl"}`)
	step("PUT", "/api/v1/assurance", "/api/v1/assurance", `{}`)

	step("POST", "/api/v1/groups", "/api/v1/groups", `{"name":"wave-1","parent":"pilot"}`)
	step("PATCH", "/api/v1/groups/{name}", "/api/v1/groups/wave-1", `{"idpGroup":"it-staff"}`)
	step("POST", "/api/v1/access", "/api/v1/access", `{"group":"it-staff","role":"viewer","scope":"group:pilot"}`)
	step("GET", "/api/v1/access", "/api/v1/access", "")
	step("DELETE", "/api/v1/access", "/api/v1/access", `{"group":"it-staff","scope":"group:pilot"}`)

	step("POST", "/api/v1/devices", "/api/v1/devices", `{"tag":"lt-2","hardware":"hp-g4","groups":["pilot"]}`)
	step("PATCH", "/api/v1/devices/{tag}", "/api/v1/devices/lt-2", `{"assignedUser":"jan"}`)
	step("POST", "/api/v1/devices/{tag}/credential", "/api/v1/devices/lt-2/credential", "")
	step("POST", "/api/v1/devices/{tag}/intent", "/api/v1/devices/lt-2/intent", `{"intent":"lock"}`)
	step("DELETE", "/api/v1/devices/{tag}/intent", "/api/v1/devices/lt-2/intent", "")
	// lt-2 is not active until it checks in; lt-1 is.
	step("POST", "/api/v1/devices/{tag}/retire", "/api/v1/devices/lt-1/retire", "")
	step("POST", "/api/v1/devices/{tag}/reactivate", "/api/v1/devices/lt-1/reactivate", "")

	step("POST", "/api/v1/settings", "/api/v1/settings", `{"scope":"org","key":"apps.bogus","value":true}`)
	step("DELETE", "/api/v1/settings", "/api/v1/settings", `{"scope":"org","key":"apps.bogus"}`)
	step("PUT", "/api/v1/apps", "/api/v1/apps", `{"scope":"org","kind":"packages","names":["htop"]}`)

	step("PUT", "/api/v1/filters/{id}", "/api/v1/filters/laptops", `{"rules":[{"attr":"class","op":"eq","value":"laptop"}]}`)
	step("PUT", "/api/v1/policies/{id}", "/api/v1/policies/office", `{"settings":{"apps.bogus":true}}`)
	step("POST", "/api/v1/assignments", "/api/v1/assignments", `{"policy":"office","target":"org","filter":"laptops"}`)
	step("DELETE", "/api/v1/assignments", "/api/v1/assignments", `{"policy":"office","target":"org","filter":"laptops"}`)
	step("DELETE", "/api/v1/policies/{id}", "/api/v1/policies/office", "")
	step("DELETE", "/api/v1/filters/{id}", "/api/v1/filters/laptops", "")

	step("POST", "/api/v1/secret-refs", "/api/v1/secret-refs", `{"name":"wifi-psk","description":"the office network"}`)
	step("GET", "/api/v1/secret-refs", "/api/v1/secret-refs", "")
	step("DELETE", "/api/v1/secret-refs/{name}", "/api/v1/secret-refs/wifi-psk", "")

	step("POST", "/api/v1/tokens", "/api/v1/tokens", `{"id":"ci","name":"CI","kind":"service","ceiling":"viewer"}`)
	step("GET", "/api/v1/tokens", "/api/v1/tokens", "")
	step("DELETE", "/api/v1/tokens/{id}", "/api/v1/tokens/ci", "")

	step("POST", "/api/v1/changes", "/api/v1/changes", `{"id":"bump-x","title":"Raise x"}`)
	step("POST", "/api/v1/changes/{id}/edits", "/api/v1/changes/bump-x/edits", `{"scope":"org","key":"x","value":2}`)
	step("GET", "/api/v1/changes/{id}/diff", "/api/v1/changes/bump-x/diff", "")
	step("POST", "/api/v1/changes/{id}/submit", "/api/v1/changes/bump-x/submit", "")
	step("POST", "/api/v1/changes/{id}/merge", "/api/v1/changes/bump-x/merge", "")
	step("GET", "/api/v1/changes/{id}", "/api/v1/changes/bump-x", "")
	step("POST", "/api/v1/changes", "/api/v1/changes", `{"id":"not-now","title":"Not now"}`)
	step("POST", "/api/v1/changes/{id}/abandon", "/api/v1/changes/not-now/abandon", "")
	step("GET", "/api/v1/changes", "/api/v1/changes", "")

	step("PUT", "/api/v1/rollout/plan", "/api/v1/rollout/plan", `{"plan":{"rings":[{"group":"pilot"}]}}`)
	_, _, audit := do("GET", "/api/v1/audit", "")
	var log []struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(audit, &log); err != nil || len(log) == 0 {
		t.Fatalf("no commit to roll out: %s", audit)
	}
	step("POST", "/api/v1/rollout", "/api/v1/rollout", `{"target":"`+log[0].Hash+`"}`)
	step("GET", "/api/v1/rollout", "/api/v1/rollout", "")
	step("POST", "/api/v1/rollout/tick", "/api/v1/rollout/tick", "")
	step("DELETE", "/api/v1/rollout", "/api/v1/rollout", "")

	// Every remaining read, against what the walk left behind.
	fill := strings.NewReplacer("{tag}", "lt-1", "{name}", "pilot", "{id}", "bump-x")
	for path, ops := range spec.Paths {
		if _, ok := ops["get"]; ok && !walked["GET "+path] {
			step("GET", path, fill.Replace(path), "")
		}
	}

	step("DELETE", "/api/v1/groups/{name}", "/api/v1/groups/wave-1", "")
	step("DELETE", "/api/v1/devices/{tag}", "/api/v1/devices/lt-2", "")

	for path, ops := range spec.Paths {
		for method := range ops {
			if op := strings.ToUpper(method) + " " + path; !walked[op] {
				t.Errorf("%s is not walked; add it to the walk so its answer is held to the spec", op)
			}
		}
	}
}

// answer is what a handler gave writeJSON.
type answer struct {
	status int
	v      any
}

// answerCheck holds a value a handler wrote to the schema documenting it.
// It walks the value, not only its type: a map of any carries a device or
// a rollout state, and those are checked as what they are.
type answerCheck struct {
	t       *testing.T
	schemas map[string]bodySchema
}

var (
	timeType      = reflect.TypeFor[time.Time]()
	marshalerType = reflect.TypeFor[json.Marshaler]()
)

func (c answerCheck) value(where string, v reflect.Value, s bodySchema) {
	c.t.Helper()
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return // null; whether it may be is the schema's description to say
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return
	}
	if s.Ref != "" {
		ref, ok := c.schemas[strings.TrimPrefix(s.Ref, "#/components/schemas/")]
		if !ok {
			c.t.Errorf("%s: %s resolves to nothing", where, s.Ref)
			return
		}
		s = ref
	}
	if s.Type == "" && len(s.Properties) == 0 {
		return // no type is any JSON value, the way a setting's value is
	}
	ty := v.Type()
	switch {
	case ty == timeType:
		if s.Type != "string" {
			c.t.Errorf("%s is a time, documented as %q (want \"string\")", where, s.Type)
		}
		return
	case ty.Implements(marshalerType) || reflect.PointerTo(ty).Implements(marshalerType):
		return // it says itself how it looks; the schema may say anything
	}
	want := map[reflect.Kind]string{reflect.String: "string", reflect.Bool: "boolean",
		reflect.Int: "integer", reflect.Int64: "integer", reflect.Float64: "number",
		reflect.Slice: "array", reflect.Map: "object", reflect.Struct: "object"}[ty.Kind()]
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
		if v.Len() == 0 {
			c.value(where+"[]", reflect.New(ty.Elem()).Elem(), *s.Items)
		}
		for i := range v.Len() {
			c.value(where+"[]", v.Index(i), *s.Items)
		}
	case reflect.Map:
		switch {
		case len(s.Properties) > 0:
			for _, k := range v.MapKeys() {
				name := k.String()
				p, ok := s.Properties[name]
				if !ok {
					c.t.Errorf("%s: the server sends %q, the spec does not document it", where, name)
					continue
				}
				c.described(where, name, p)
				c.value(where+"."+name, v.MapIndex(k), p)
			}
			for _, r := range s.Required {
				if !v.MapIndex(reflect.ValueOf(r)).IsValid() {
					c.t.Errorf("%s: the spec requires %q and the server did not send it", where, r)
				}
			}
		case len(s.AdditionalProperties) > 0:
			var values bodySchema
			if err := json.Unmarshal(s.AdditionalProperties, &values); err != nil {
				return // true: any value
			}
			for _, k := range v.MapKeys() {
				c.value(where+"{}", v.MapIndex(k), values)
			}
		default:
			if strings.TrimSpace(s.Description) == "" {
				c.t.Errorf("%s: an object with no fields documented has to say what it is", where)
			}
		}
	case reflect.Struct:
		if len(s.Properties) == 0 {
			if strings.TrimSpace(s.Description) == "" {
				c.t.Errorf("%s: an object with no fields documented has to say what it is", where)
			}
			return
		}
		fields := answerFields(ty)
		for name, f := range fields {
			p, ok := s.Properties[name]
			if !ok {
				c.t.Errorf("%s: the server sends %q, the spec does not document it", where, name)
				continue
			}
			c.described(where, name, p)
			c.value(where+"."+name, v.FieldByIndex(f.index), p)
		}
		for name := range s.Properties {
			if _, ok := fields[name]; !ok {
				c.t.Errorf("%s: the spec documents %q, which the server never sends", where, name)
			}
		}
		for _, r := range s.Required {
			if f, ok := fields[r]; ok && f.omitempty {
				c.t.Errorf("%s: the spec says %q is always there, and the server leaves it out when it is empty", where, r)
			}
		}
	}
}

func (c answerCheck) described(where, name string, p bodySchema) {
	if p.Ref == "" && strings.TrimSpace(p.Description) == "" {
		c.t.Errorf("%s.%s does not say what it means", where, name)
	}
}

type answerField struct {
	index     []int
	omitempty bool
}

// answerFields is what encoding/json writes from a struct.
func answerFields(ty reflect.Type) map[string]answerField {
	out := map[string]answerField{}
	for i := range ty.NumField() {
		f := ty.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			for n, inner := range answerFields(f.Type) {
				inner.index = append([]int{i}, inner.index...)
				out[n] = inner
			}
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = answerField{index: []int{i}, omitempty: strings.Contains(","+opts+",", ",omitempty,")}
	}
	return out
}
