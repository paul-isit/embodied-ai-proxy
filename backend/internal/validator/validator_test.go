package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestValidator(t *testing.T) *Validator {
	t.Helper()
	path, err := filepath.Abs("../../../configs/json_schema.json")
	if err != nil {
		t.Fatalf("resolve schema path: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema file: %v", err)
	}
	v, err := New(path, raw)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return v
}

func TestValidator_ValidSuccessRecipe(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Pick and place",
		"steps": [
			{"step_id": 1, "action": "home", "description": "go home", "parameters": {}},
			{"step_id": 2, "action": "move_arm", "description": "move", "parameters": {"target": "red_cube"}}
		]
	}`)
	if _, err := v.Validate(raw); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_ValidErrorRecipe(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{"status": "error", "error_type": "missing_object", "message": "no such object"}`)
	if _, err := v.Validate(raw); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_RejectsUnknownAction(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Bad routine",
		"steps": [
			{"step_id": 1, "action": "fly", "description": "not a real action", "parameters": {}}
		]
	}`)
	if _, err := v.Validate(raw); err == nil {
		t.Error("Validate() error = nil, want error for unknown action")
	}
}

func TestValidator_RejectsMissingRequiredField(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{"status": "success", "steps": []}`)
	if _, err := v.Validate(raw); err == nil {
		t.Error("Validate() error = nil, want error for missing recipe_name")
	}
}

func TestValidator_RejectsMalformedJSON(t *testing.T) {
	v := newTestValidator(t)

	if _, err := v.Validate([]byte(`not json`)); err == nil {
		t.Error("Validate() error = nil, want error for malformed JSON")
	}
}
