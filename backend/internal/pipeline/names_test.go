package pipeline

import (
	"embodied-ai-proxy/backend/internal/rosbridge"
	"encoding/json"
	"strings"
	"testing"
)

func testEnvironment() rosbridge.EnvironmentParams {
	return rosbridge.EnvironmentParams{
		Objects:      []string{"blue_cube", "delivery_tray"},
		Movements:    []string{"move_upwards"},
		Orientations: []string{"facing_forward"},
	}
}

func decodeDoc(t *testing.T, raw string) any {
	t.Helper()
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("failed to decode fixture JSON: %v", err)
	}
	return doc
}

func TestResolveRecipeNames_RewritesTypo(t *testing.T) {
	doc := decodeDoc(t, `{
		"status": "success",
		"recipe_name": "Test",
		"steps": [
			{"step_id": 1, "action": "pickup", "description": "d", "parameters": {"target": "blu cube"}}
		]
	}`)

	resolved, raw, err := resolveRecipeNames(doc, testEnvironment())
	if err != nil {
		t.Fatalf("resolveRecipeNames() error = %v, want nil", err)
	}
	if !strings.Contains(string(raw), `"blue_cube"`) {
		t.Errorf("raw = %s, want target rewritten to blue_cube", raw)
	}
	root := resolved.(map[string]any)
	steps := root["steps"].([]any)
	params := steps[0].(map[string]any)["parameters"].(map[string]any)
	if params["target"] != "blue_cube" {
		t.Errorf("target = %v, want blue_cube", params["target"])
	}
}

func TestResolveRecipeNames_OmittedOptionalFieldIsNotAnError(t *testing.T) {
	// dropoff with no 'target' at all - the held-object fallback case -
	// must be skipped over, not treated as malformed input.
	doc := decodeDoc(t, `{
		"status": "success",
		"recipe_name": "Test",
		"steps": [
			{"step_id": 1, "action": "dropoff", "description": "d", "parameters": {"destination": "delivery_tray"}}
		]
	}`)

	resolved, _, err := resolveRecipeNames(doc, testEnvironment())
	if err != nil {
		t.Fatalf("resolveRecipeNames() error = %v, want nil", err)
	}
	if recipeStatus(resolved) != "success" {
		t.Errorf("status = %v, want success", recipeStatus(resolved))
	}
}

func TestResolveRecipeNames_OmittedParametersObjectIsNotAnError(t *testing.T) {
	// 'home' has nothing but an optional 'speed', so the schema doesn't
	// require 'parameters' at all for it (unlike every other action) - a
	// bare 'home' step omits 'parameters' entirely. This must be skipped
	// over, not treated as a malformed step.
	doc := decodeDoc(t, `{
		"status": "success",
		"recipe_name": "Test",
		"steps": [
			{"step_id": 1, "action": "home", "description": "Start at home"},
			{"step_id": 2, "action": "pickup", "description": "d", "parameters": {"target": "blue_cube"}},
			{"step_id": 3, "action": "home", "description": "Return to home"}
		]
	}`)

	resolved, _, err := resolveRecipeNames(doc, testEnvironment())
	if err != nil {
		t.Fatalf("resolveRecipeNames() error = %v, want nil", err)
	}
	if recipeStatus(resolved) != "success" {
		t.Errorf("status = %v, want success", recipeStatus(resolved))
	}
}

func TestResolveRecipeNames_UnresolvableNameRejectsWithoutError(t *testing.T) {
	doc := decodeDoc(t, `{
		"status": "success",
		"recipe_name": "Test",
		"steps": [
			{"step_id": 1, "action": "pickup", "description": "d", "parameters": {"target": "nonexistent_widget"}}
		]
	}`)

	resolved, _, err := resolveRecipeNames(doc, testEnvironment())
	if err != nil {
		t.Fatalf("resolveRecipeNames() error = %v, want nil (rejection is a recipe-level error, not a Go error)", err)
	}
	if recipeStatus(resolved) != "error" || recipeErrorType(resolved) != "missing_object" {
		t.Errorf("resolved = %+v, want status=error error_type=missing_object", resolved)
	}
}

func TestResolveRecipeNames_AmbiguousNameRejectsWithoutError(t *testing.T) {
	doc := decodeDoc(t, `{
		"status": "success",
		"recipe_name": "Test",
		"steps": [
			{"step_id": 1, "action": "pickup", "description": "d", "parameters": {"target": "cube"}}
		]
	}`)
	env := rosbridge.EnvironmentParams{Objects: []string{"blue_cube", "red_cube"}}

	resolved, _, err := resolveRecipeNames(doc, env)
	if err != nil {
		t.Fatalf("resolveRecipeNames() error = %v, want nil", err)
	}
	if recipeStatus(resolved) != "error" || recipeErrorType(resolved) != "invalid_command" {
		t.Errorf("resolved = %+v, want status=error error_type=invalid_command", resolved)
	}
}

func TestResolveRecipeNames_StepsNotArrayReturnsError(t *testing.T) {
	doc := decodeDoc(t, `{"status": "success", "recipe_name": "Test", "steps": "not-an-array"}`)

	if _, _, err := resolveRecipeNames(doc, testEnvironment()); err == nil {
		t.Error("resolveRecipeNames() error = nil, want error for malformed 'steps'")
	}
}

func TestResolveRecipeNames_StepNotObjectReturnsError(t *testing.T) {
	doc := decodeDoc(t, `{"status": "success", "recipe_name": "Test", "steps": ["not-an-object"]}`)

	if _, _, err := resolveRecipeNames(doc, testEnvironment()); err == nil {
		t.Error("resolveRecipeNames() error = nil, want error for a step that isn't an object")
	}
}

func TestResolveRecipeNames_ParametersNotObjectReturnsError(t *testing.T) {
	doc := decodeDoc(t, `{
		"status": "success",
		"recipe_name": "Test",
		"steps": [
			{"step_id": 1, "action": "home", "description": "d", "parameters": "not-an-object"}
		]
	}`)

	if _, _, err := resolveRecipeNames(doc, testEnvironment()); err == nil {
		t.Error("resolveRecipeNames() error = nil, want error for non-object 'parameters'")
	}
}

func TestResolveRecipeNames_NonStringFieldValueReturnsError(t *testing.T) {
	doc := decodeDoc(t, `{
		"status": "success",
		"recipe_name": "Test",
		"steps": [
			{"step_id": 1, "action": "pickup", "description": "d", "parameters": {"target": 42}}
		]
	}`)

	if _, _, err := resolveRecipeNames(doc, testEnvironment()); err == nil {
		t.Error("resolveRecipeNames() error = nil, want error for a non-string 'target'")
	}
}

func TestResolveRecipeNames_DocNotObjectReturnsError(t *testing.T) {
	doc := decodeDoc(t, `["not", "an", "object"]`)

	if _, _, err := resolveRecipeNames(doc, testEnvironment()); err == nil {
		t.Error("resolveRecipeNames() error = nil, want error for a non-object document")
	}
}
