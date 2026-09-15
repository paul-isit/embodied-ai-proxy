package app

import (
	"embodied-ai-proxy/tui/internal/client"
	"strings"
	"testing"
	"time"
)

func TestFormatActionRecipeSuccessShowsSteps(t *testing.T) {
	payload := []byte(`{"status":"success","recipe_name":"Pick Up","steps":[{"step_id":1,"action":"pickup","description":"Grab it","parameters":{"target":"red_cube"}}]}`)

	got := formatActionRecipe(payload, 1, 0)

	if !strings.Contains(got, "Validated Robot Recipe: Pick Up") {
		t.Fatalf("expected recipe name in output, got %s", got)
	}
	if !strings.Contains(got, "1. pickup - Grab it") {
		t.Fatalf("expected step line in output, got %s", got)
	}
}

func TestFormatActionRecipeErrorShowsMessage(t *testing.T) {
	payload := []byte(`{"status":"error","error_type":"missing_object","message":"no such object"}`)

	got := formatActionRecipe(payload, 1, 0)

	if !strings.Contains(got, "Schema parsing failure (missing_object)") {
		t.Fatalf("expected error type in output, got %s", got)
	}
	if !strings.Contains(got, "no such object") {
		t.Fatalf("expected error message in output, got %s", got)
	}
}

func TestFormatActionRecipeMalformedPayloadReturnsError(t *testing.T) {
	got := formatActionRecipe([]byte(`not json`), 1, 0)

	if !strings.Contains(got, "failed to parse action_recipe") {
		t.Fatalf("expected parse-error message, got %s", got)
	}
}

func TestFormatActionRecipeVerbosityGatesDetail(t *testing.T) {
	payload := []byte(`{"status":"success","recipe_name":"Pick Up","steps":[{"step_id":1,"action":"pickup","description":"Grab it","parameters":{"target":"red_cube"}}]}`)

	l1 := formatActionRecipe(payload, 1, 0)
	if strings.Contains(l1, "params:") || strings.Contains(l1, "RAW JSON") {
		t.Fatalf("L1 should not show params or raw JSON, got %s", l1)
	}

	l2 := formatActionRecipe(payload, 2, 0)
	if !strings.Contains(l2, "params:") || !strings.Contains(l2, "RAW JSON") {
		t.Fatalf("L2 should show params and raw JSON, got %s", l2)
	}
	if strings.Contains(l2, "METADATA") {
		t.Fatalf("L2 should not show metadata, got %s", l2)
	}

	l3 := formatActionRecipe(payload, 3, 250*time.Millisecond)
	if !strings.Contains(l3, "METADATA") || !strings.Contains(l3, "Latency: 250ms") {
		t.Fatalf("L3 should show metadata with latency, got %s", l3)
	}
}

func TestFormatLogEventTagsByLevel(t *testing.T) {
	errPayload := []byte(`{"level":"error","message":"upstream failed"}`)
	got := formatLogEvent(errPayload)
	if !strings.Contains(got, "upstream failed") {
		t.Fatalf("expected message in output, got %s", got)
	}
	if !isErrorLevel(errPayload) {
		t.Fatalf("expected isErrorLevel to be true for level=error")
	}

	infoPayload := []byte(`{"level":"info","message":"heartbeat"}`)
	if isErrorLevel(infoPayload) {
		t.Fatalf("expected isErrorLevel to be false for level=info")
	}
}

func TestFormatLogEventMalformedPayloadReturnsError(t *testing.T) {
	got := formatLogEvent([]byte(`not json`))
	if !strings.Contains(got, "failed to parse log_event") {
		t.Fatalf("expected parse-error message, got %s", got)
	}
}

func TestStateLabelAndStyle(t *testing.T) {
	cases := []struct {
		state int
		want  string
	}{
		{StateReady, "READY"},
		{StateBusy, "BUSY"},
		{StateFault, "FAULT"},
	}
	for _, c := range cases {
		if got := stateLabel(c.state); got != c.want {
			t.Errorf("stateLabel(%d) = %q, want %q", c.state, got, c.want)
		}
	}
}

func TestContentWidthNeverGoesBelowOne(t *testing.T) {
	if got := contentWidth(0); got != 1 {
		t.Fatalf("expected contentWidth(0) to floor at 1, got %d", got)
	}
	if got := contentWidth(3); got != 1 {
		t.Fatalf("expected contentWidth(3) to floor at 1, got %d", got)
	}
}

func TestBridgeStatusText(t *testing.T) {
	connected := true
	disconnected := false

	if got := bridgeStatusText(nil); got != "bridge: unknown" {
		t.Errorf("bridgeStatusText(nil) = %q", got)
	}
	if got := bridgeStatusText(&connected); got != "bridge: connected" {
		t.Errorf("bridgeStatusText(true) = %q", got)
	}
	if got := bridgeStatusText(&disconnected); got != "bridge: disconnected" {
		t.Errorf("bridgeStatusText(false) = %q", got)
	}
}

func TestStatusUpdateEnvelopeUpdatesObjectList(t *testing.T) {
	m := NewModel("http://localhost:8080", "")
	updated, _ := m.Update(client.Envelope{
		Type:    client.TypeStatusUpdate,
		Payload: []byte(`{"object_list": ["red_cube", "green_apple"]}`),
	})
	nm := updated.(Model)

	if len(nm.availableObjects) != 2 || nm.availableObjects[0] != "red_cube" || nm.availableObjects[1] != "green_apple" {
		t.Fatalf("expected availableObjects to be updated, got %v", nm.availableObjects)
	}

	nm.Ready = true
	view := nm.View()
	if !strings.Contains(view, "red_cube, green_apple") {
		t.Fatalf("expected view to contain objects, got %s", view)
	}
}