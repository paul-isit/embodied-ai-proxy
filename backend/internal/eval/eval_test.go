package eval

import (
	"bytes"
	"context"
	"embodied-ai-proxy/backend/internal/pipeline"
	"embodied-ai-proxy/backend/internal/validator"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode %s: %v", s, err)
	}
	return v
}

func TestMatch(t *testing.T) {
	got := `{"status":"success","recipe_name":"x","steps":[
		{"step_id":1,"action":"home","description":"d"},
		{"step_id":2,"action":"pour","description":"d","parameters":{"tilt_angle":0.60}}]}`

	for _, tc := range []struct {
		want, mismatch string
	}{
		{`{"status":"success"}`, ""},
		{`{"steps":[{"action":"home"},{"action":"pour","parameters":{"tilt_angle":0.6}}]}`, ""},
		{`{"steps":[{"action":"home"},{"parameters":{"target":null}}]}`, ""},
		{`{"status":"error"}`, `status: want "error", got "success"`},
		{`{"steps":[{"action":"home"}]}`, "steps: want 1 items, got 2"},
		{`{"steps":[{},{"parameters":{"target":"mug"}}]}`, `steps[1].parameters.target: missing, want "mug"`},
		{`{"steps":[{},{"parameters":{"tilt_angle":null}}]}`, "steps[1].parameters.tilt_angle: want absent, got 0.60"},
		{`{"steps":[{},{"parameters":{"tilt_angle":1}}]}`, "steps[1].parameters.tilt_angle: want 1, got 0.60"},
	} {
		if mismatch := match("", decode(t, tc.want), decode(t, got)); mismatch != tc.mismatch {
			t.Errorf("match(%s) = %q, want %q", tc.want, mismatch, tc.mismatch)
		}
	}
}

// TestRun runs a suite through the real pipeline against a fake LLM proxy.
func TestRun(t *testing.T) {
	fakeLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Prompt string }
		json.NewDecoder(r.Body).Decode(&request)
		reply := `{"status":"success","recipe_name":"r","steps":[{"step_id":1,"action":"pickup","description":"d","parameters":{"target":"blu cube"}}]}`
		switch {
		case strings.HasSuffix(request.Prompt, "car"):
			reply = `{"status":"error","error_type":"missing_object","message":"no car"}`
		case strings.HasSuffix(request.Prompt, "junk"):
			reply = "sorry, I can't"
		}
		json.NewEncoder(w).Encode(map[string]string{"text": reply})
	}))
	defer fakeLLM.Close()

	schemaPath, _ := filepath.Abs("../../../data/config/json_schema.json")
	schemaRaw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	schemaValidator, err := validator.New(schemaPath, schemaRaw)
	if err != nil {
		t.Fatal(err)
	}
	promptPipeline := pipeline.New(nil, nil, schemaValidator, fakeLLM.URL, "{user_command}", schemaRaw)

	suitePath := filepath.Join(t.TempDir(), "suite.json")
	os.WriteFile(suitePath, []byte(`{
		"environment": {"objects": ["blue_cube", "delivery_tray"]},
		"cases": [
			{"name": "typo resolved", "prompt": "pick up cube",
			 "expect": [{"status": "success", "steps": [{"action": "pickup", "parameters": {"target": "blue_cube"}}]}]},
			{"name": "wrong target", "prompt": "pick up cube",
			 "expect": [{"steps": [{"parameters": {"target": "delivery_tray"}}]}]},
			{"name": "refusal", "prompt": "pick up car", "expect": [{"status": "error", "error_type": "missing_object"}]},
			{"name": "not json", "prompt": "junk"},
			{"name": "valid only", "prompt": "pick up cube"}
		]
	}`), 0o644)

	var out bytes.Buffer
	failed := Run(context.Background(), promptPipeline, "fake · model", []string{suitePath, "missing.json"}, &out, nil)

	report := out.String()
	if failed != 3 { // wrong target, not json, missing.json
		t.Errorf("Run() = %d failures, want 3\n%s", failed, report)
	}
	for _, want := range []string{
		"Evaluating fake · model",
		"PASS typo resolved",
		"FAIL wrong target",
		"want    *(target=delivery_tray)",
		`↳ steps[0].parameters.target: want "delivery_tray", got "blue_cube"`,
		"got     pickup(target=blue_cube)",
		"PASS refusal",
		"FAIL not json",
		"schema validation failed",
		"raw     sorry, I can't",
		"PASS valid only",
		"open missing.json",
		"2 passed · 3 failed · 5 cases",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q\n%s", want, report)
		}
	}
}

func TestRun_WritesResultsAndPassesHeldObjects(t *testing.T) {
	var gotPrompt string
	fakeLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Prompt string }
		json.NewDecoder(r.Body).Decode(&request)
		gotPrompt = request.Prompt
		reply := `{"status":"success","recipe_name":"r","steps":[{"step_id":1,"action":"dropoff","description":"d","parameters":{}}]}`
		json.NewEncoder(w).Encode(map[string]string{"text": reply})
	}))
	defer fakeLLM.Close()

	schemaPath, _ := filepath.Abs("../../../data/config/json_schema.json")
	schemaRaw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	schemaValidator, err := validator.New(schemaPath, schemaRaw)
	if err != nil {
		t.Fatal(err)
	}
	promptPipeline := pipeline.New(nil, nil, schemaValidator, fakeLLM.URL, "held: {held_objects} | {user_command}", schemaRaw)

	suitePath := filepath.Join(t.TempDir(), "suite.json")
	os.WriteFile(suitePath, []byte(`{
		"environment": {"objects": ["blue_cube"], "held_objects": ["blue_cube"]},
		"cases": [{"name": "put it down", "prompt": "put it down"}]
	}`), 0o644)

	var out, results bytes.Buffer
	if failed := Run(context.Background(), promptPipeline, "fake · model", []string{suitePath}, &out, &results); failed != 0 {
		t.Fatalf("Run() = %d failures\n%s", failed, out.String())
	}

	if !strings.Contains(gotPrompt, "- blue_cube") {
		t.Errorf("prompt = %q, want it to list the held object", gotPrompt)
	}
	var line struct {
		Name   string          `json:"name"`
		Passed bool            `json:"passed"`
		Parsed json.RawMessage `json:"parsed"`
	}
	if err := json.Unmarshal(results.Bytes(), &line); err != nil {
		t.Fatalf("results = %q: %v", results.String(), err)
	}
	if line.Name != "put it down" || !line.Passed || !strings.Contains(string(line.Parsed), `"dropoff"`) {
		t.Errorf("results line = %+v, want the passed dropoff recipe", line)
	}
}
