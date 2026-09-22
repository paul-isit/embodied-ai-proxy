package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestValidator(t *testing.T) *Validator {
	t.Helper()
	path, err := filepath.Abs("../../../data/config/json_schema.json")
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

func TestValidator_PushAcceptsDestination(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Push",
		"steps": [
			{"step_id": 1, "action": "push", "description": "push it", "parameters": {"target": "red_cube", "destination": "delivery_tray"}}
		]
	}`)
	if _, err := v.Validate(raw); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_PushAcceptsDirection(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Push",
		"steps": [
			{"step_id": 1, "action": "push", "description": "push it", "parameters": {"target": "red_cube", "direction": "forward", "distance": 0.3}}
		]
	}`)
	if _, err := v.Validate(raw); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_PushRejectsNeitherDestinationNorDirection(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Push",
		"steps": [
			{"step_id": 1, "action": "push", "description": "push it", "parameters": {"target": "red_cube"}}
		]
	}`)
	if _, err := v.Validate(raw); err == nil {
		t.Error("Validate() error = nil, want error for push with neither destination nor direction")
	}
}

func TestValidator_PushRejectsInvalidDirection(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Push",
		"steps": [
			{"step_id": 1, "action": "push", "description": "push it", "parameters": {"target": "red_cube", "direction": "sideways"}}
		]
	}`)
	if _, err := v.Validate(raw); err == nil {
		t.Error("Validate() error = nil, want error for invalid direction enum value")
	}
}

func TestValidator_ThrowAcceptsDirection(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Throw",
		"steps": [
			{"step_id": 1, "action": "throw", "description": "throw it", "parameters": {"target": "red_cube", "direction": "left"}}
		]
	}`)
	if _, err := v.Validate(raw); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_ThrowRejectsNeitherDestinationNorDirection(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Throw",
		"steps": [
			{"step_id": 1, "action": "throw", "description": "throw it", "parameters": {"target": "red_cube"}}
		]
	}`)
	if _, err := v.Validate(raw); err == nil {
		t.Error("Validate() error = nil, want error for throw with neither destination nor direction")
	}
}

func TestValidator_PourAcceptsLiftHeightAndTiltAngle(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Pour",
		"steps": [
			{"step_id": 1, "action": "pour", "description": "pour it", "parameters": {"target": "mug", "destination": "bowl", "lift_height": 0.14, "tilt_angle": 2.356, "duration": 1.5}}
		]
	}`)
	if _, err := v.Validate(raw); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_PourRejectsOldAmountField(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Pour",
		"steps": [
			{"step_id": 1, "action": "pour", "description": "pour it", "parameters": {"target": "mug", "destination": "bowl", "amount": 0.5}}
		]
	}`)
	if _, err := v.Validate(raw); err == nil {
		t.Error("Validate() error = nil, want error for pour's old removed 'amount' field")
	}
}

func TestValidator_PourRequiresDestinationOrDirection(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Pour",
		"steps": [
			{"step_id": 1, "action": "pour", "description": "pour it", "parameters": {"target": "mug"}}
		]
	}`)
	if _, err := v.Validate(raw); err == nil {
		t.Error("Validate() error = nil, want error for pour with neither destination nor direction")
	}
}

func TestValidator_ThrowAcceptsSpeedAndOpenPosition(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Throw",
		"steps": [
			{"step_id": 1, "action": "throw", "description": "throw it", "parameters": {"target": "red_cube", "direction": "forward", "distance": 0.3, "open_position": 0.0, "speed": 0.6}}
		]
	}`)
	if _, err := v.Validate(raw); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_ThrowRejectsOldWindUpFields(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Throw",
		"steps": [
			{"step_id": 1, "action": "throw", "description": "throw it", "parameters": {"target": "red_cube", "direction": "forward", "wind_up_angle": 0.5, "fling_angle": 1.0, "release_delay": 0.3}}
		]
	}`)
	if _, err := v.Validate(raw); err == nil {
		t.Error("Validate() error = nil, want error for throw's old removed wind_up_angle/fling_angle/release_delay fields")
	}
}

func TestValidator_PickupAcceptsGraspStyleSide(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Pickup",
		"steps": [
			{"step_id": 1, "action": "pickup", "description": "pick it up", "parameters": {"target": "red_cube", "grasp_style": "side"}}
		]
	}`)
	if _, err := v.Validate(raw); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_PickupAcceptsManualOrientationAndGraspOffset(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Pickup",
		"steps": [
			{"step_id": 1, "action": "pickup", "description": "pick it up", "parameters": {"target": "red_cube", "orientation": "facing_forward", "grasp_offset": {"x": -0.04, "y": 0.0, "z": 0.01}}}
		]
	}`)
	if _, err := v.Validate(raw); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_PickupRejectsUnknownGraspStyle(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Pickup",
		"steps": [
			{"step_id": 1, "action": "pickup", "description": "pick it up", "parameters": {"target": "red_cube", "grasp_style": "top"}}
		]
	}`)
	if _, err := v.Validate(raw); err == nil {
		t.Error("Validate() error = nil, want error for an unknown grasp_style")
	}
}

func TestValidator_ThrustAcceptsLiftHeightAndDestination(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Thrust",
		"steps": [
			{"step_id": 1, "action": "thrust", "description": "thrust it", "parameters": {"target": "red_cube", "destination": "delivery_tray", "lift_height": 0.14, "speed": 1.0}}
		]
	}`)
	if _, err := v.Validate(raw); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_ThrustRequiresDestinationOrDirection(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Thrust",
		"steps": [
			{"step_id": 1, "action": "thrust", "description": "thrust it", "parameters": {"target": "red_cube"}}
		]
	}`)
	if _, err := v.Validate(raw); err == nil {
		t.Error("Validate() error = nil, want error for thrust with neither destination nor direction")
	}
}

func TestValidator_ThrustRejectsOldVectorAndOrientationFields(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Thrust",
		"steps": [
			{"step_id": 1, "action": "thrust", "description": "thrust it", "parameters": {"target": "red_cube", "direction": "forward", "orientation": "facing_forward", "vector": "thrust_forward"}}
		]
	}`)
	if _, err := v.Validate(raw); err == nil {
		t.Error("Validate() error = nil, want error for thrust's old removed orientation/vector fields")
	}
}

func TestValidator_PushRejectsOldOrientationAndHeightOffsetFields(t *testing.T) {
	v := newTestValidator(t)

	raw := []byte(`{
		"status": "success",
		"recipe_name": "Push",
		"steps": [
			{"step_id": 1, "action": "push", "description": "push it", "parameters": {"target": "red_cube", "direction": "forward", "orientation": "facing_forward", "height_offset": 0.01}}
		]
	}`)
	if _, err := v.Validate(raw); err == nil {
		t.Error("Validate() error = nil, want error for push's old removed orientation/height_offset fields")
	}
}
