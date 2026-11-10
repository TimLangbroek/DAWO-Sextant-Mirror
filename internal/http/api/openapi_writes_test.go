package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// The spec said 200 for four writes that answer 201, and nothing caught it:
// the contract tests looked at what the GETs answer and never made a write
// succeed. A generated client that trusts the spec treats a 201 it was not
// told about as a failure.
//
// This walks every write in the spec with a body the spec documents, in an
// order where each finds what it needs, and holds every answer to the
// spec: the status is one the operation documents, and a documented object
// carries its required fields. It also proves the documented bodies are
// ones the server takes, not only ones it decodes.
func TestEveryWriteAnswersAsDocumented(t *testing.T) {
	mux := http.NewServeMux()
	fullAPI(t, mux)
	srv := httptest.NewServer(mux)
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

	do := func(method, path, body string) (int, []byte) {
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
		return resp.StatusCode, b
	}

	walked := map[string]bool{}
	step := func(method, pattern, path, body string) []byte {
		t.Helper()
		op, ok := spec.Paths[pattern][strings.ToLower(method)]
		if !ok {
			t.Fatalf("%s %s is not in the spec", method, pattern)
		}
		walked[method+" "+pattern] = true
		code, got := do(method, path, body)
		if code < 200 || code > 299 {
			t.Fatalf("%s %s with %s: %d %s", method, path, body, code, got)
		}
		answer, ok := op.Responses[strconv.Itoa(code)]
		if !ok {
			t.Errorf("%s %s answers %d, which the spec does not document", method, pattern, code)
			return got
		}
		schema := answer.Content["application/json"].Schema
		if schema.Ref != "" {
			schema = spec.Components.Schemas[strings.TrimPrefix(schema.Ref, "#/components/schemas/")]
		}
		if schema.Type == "object" && len(schema.Required) > 0 {
			var m map[string]any
			if err := json.Unmarshal(got, &m); err != nil {
				t.Errorf("%s %s: the spec documents an object, the server sent %s", method, pattern, got)
				return got
			}
			for _, k := range schema.Required {
				if _, ok := m[k]; !ok {
					t.Errorf("%s %s: the spec requires %q in the answer and the server did not send it", method, pattern, k)
				}
			}
		}
		return got
	}

	step("PUT", "/api/v1/me/preferences", "/api/v1/me/preferences", `{"timezone":"Europe/Amsterdam","locale":"nl"}`)
	step("PUT", "/api/v1/assurance", "/api/v1/assurance", `{}`)

	step("POST", "/api/v1/groups", "/api/v1/groups", `{"name":"wave-1","parent":"pilot"}`)
	step("PATCH", "/api/v1/groups/{name}", "/api/v1/groups/wave-1", `{"idpGroup":"it-staff"}`)
	step("POST", "/api/v1/access", "/api/v1/access", `{"group":"it-staff","role":"viewer","scope":"group:pilot"}`)
	step("DELETE", "/api/v1/access", "/api/v1/access", `{"group":"it-staff","scope":"group:pilot"}`)

	step("POST", "/api/v1/devices", "/api/v1/devices", `{"tag":"lt-2","hardware":"hp-g4","groups":["pilot"]}`)
	step("PATCH", "/api/v1/devices/{tag}", "/api/v1/devices/lt-2", `{"assignedUser":"jan"}`)
	step("POST", "/api/v1/devices/{tag}/credential", "/api/v1/devices/lt-2/credential", "")
	step("POST", "/api/v1/devices/{tag}/intent", "/api/v1/devices/lt-2/intent", `{"intent":"lock"}`)
	step("DELETE", "/api/v1/devices/{tag}/intent", "/api/v1/devices/lt-2/intent", "")
	step("POST", "/api/v1/devices/{tag}/retire", "/api/v1/devices/lt-1/retire", "") // lt-2 is not active until it checks in
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
	step("DELETE", "/api/v1/secret-refs/{name}", "/api/v1/secret-refs/wifi-psk", "")

	step("POST", "/api/v1/tokens", "/api/v1/tokens", `{"id":"ci","name":"CI","kind":"service","ceiling":"viewer"}`)
	step("DELETE", "/api/v1/tokens/{id}", "/api/v1/tokens/ci", "")

	step("POST", "/api/v1/changes", "/api/v1/changes", `{"id":"bump-x","title":"Raise x"}`)
	step("POST", "/api/v1/changes/{id}/edits", "/api/v1/changes/bump-x/edits", `{"scope":"org","key":"x","value":2}`)
	step("POST", "/api/v1/changes/{id}/submit", "/api/v1/changes/bump-x/submit", "")
	step("POST", "/api/v1/changes/{id}/merge", "/api/v1/changes/bump-x/merge", "")
	step("POST", "/api/v1/changes", "/api/v1/changes", `{"id":"not-now","title":"Not now"}`)
	step("POST", "/api/v1/changes/{id}/abandon", "/api/v1/changes/not-now/abandon", "")

	step("PUT", "/api/v1/rollout/plan", "/api/v1/rollout/plan", `{"plan":{"rings":[{"group":"pilot"}]}}`)
	_, audit := do("GET", "/api/v1/audit", "")
	var log []struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(audit, &log); err != nil || len(log) == 0 {
		t.Fatalf("no commit to roll out: %s", audit)
	}
	step("POST", "/api/v1/rollout", "/api/v1/rollout", `{"target":"`+log[0].Hash+`"}`)
	step("POST", "/api/v1/rollout/tick", "/api/v1/rollout/tick", "")
	step("DELETE", "/api/v1/rollout", "/api/v1/rollout", "")

	step("DELETE", "/api/v1/groups/{name}", "/api/v1/groups/wave-1", "")
	step("DELETE", "/api/v1/devices/{tag}", "/api/v1/devices/lt-2", "")

	for path, ops := range spec.Paths {
		for method := range ops {
			if op := strings.ToUpper(method) + " " + path; method != "get" && !walked[op] {
				t.Errorf("%s is not walked; add it to the walk so its answer is held to the spec", op)
			}
		}
	}
}
