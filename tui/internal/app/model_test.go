package app

import (
	"embodied-ai-proxy/tui/internal/client"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWaitForWSMsgDeliversQueuedMessage(t *testing.T) {
	ch := make(chan tea.Msg, 1)
	ch <- struct{ marker string }{marker: "queued"}

	msg := waitForWSMsg(ch)()

	got, ok := msg.(struct{ marker string })
	if !ok || got.marker != "queued" {
		t.Fatalf("expected the queued message to be returned, got %#v", msg)
	}
}

func TestWaitForWSMsgBlocksUntilDelivered(t *testing.T) {
	ch := make(chan tea.Msg)
	done := make(chan tea.Msg, 1)

	go func() { done <- waitForWSMsg(ch)() }()

	select {
	case <-done:
		t.Fatal("expected waitForWSMsg to block with no message on the channel")
	case <-time.After(50 * time.Millisecond):
	}

	ch <- "hello"
	select {
	case msg := <-done:
		if msg != "hello" {
			t.Fatalf("expected %q, got %v", "hello", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delivery")
	}
}

func TestSubmitPromptIgnoresEmptyInput(t *testing.T) {
	m := NewModel("http://localhost:8080")
	updated, cmd := m.submitPrompt()

	nm := updated.(Model)
	if len(nm.entries) != 0 {
		t.Fatalf("expected no output for empty input, got %v", nm.entries)
	}
	if cmd != nil {
		t.Fatalf("expected no command for empty input")
	}
}

func TestSubmitPromptSurfacesSendErrorWhenDisconnected(t *testing.T) {
	m := NewModel("http://localhost:8080")
	m.input.SetValue("pick up the red block")

	updated, _ := m.submitPrompt()
	nm := updated.(Model)

	if nm.inFlight {
		t.Fatalf("expected inFlight to remain false when send fails")
	}
	if len(nm.entries) != 1 || !strings.Contains(nm.entries[0], "websocket is not connected") {
		t.Fatalf("expected a send-error line in output, got %v", nm.entries)
	}
}

func TestSubmitPromptIgnoresWhileInFlight(t *testing.T) {
	m := NewModel("http://localhost:8080")
	m.inFlight = true
	m.input.SetValue("another command")

	updated, cmd := m.submitPrompt()
	nm := updated.(Model)

	if len(nm.entries) != 0 {
		t.Fatalf("expected no output while a command is already in flight, got %v", nm.entries)
	}
	if cmd != nil {
		t.Fatalf("expected no command while a command is already in flight")
	}
}

func TestStatusUpdateEnvelopeUpdatesHeaderNotFeed(t *testing.T) {
	m := NewModel("http://localhost:8080")
	m.inFlight = true

	updated, _ := m.Update(client.Envelope{
		Type:    client.TypeStatusUpdate,
		Payload: []byte(`{"bridge_connected": true}`),
	})
	nm := updated.(Model)

	if len(nm.entries) != 0 {
		t.Fatalf("expected status_update to stay out of the feed, got %v", nm.entries)
	}
	if nm.bridgeConnected == nil || !*nm.bridgeConnected {
		t.Fatalf("expected bridgeConnected to be true, got %v", nm.bridgeConnected)
	}
	if !nm.inFlight {
		t.Fatalf("a status_update push should not clear an in-flight prompt")
	}
}

func TestActionRecipeEnvelopeAppendsToFeedAndClearsInFlight(t *testing.T) {
	m := NewModel("http://localhost:8080")
	m.inFlight = true
	m.viewport.Width = 80

	updated, _ := m.Update(client.Envelope{
		Type:    client.TypeActionRecipe,
		Payload: []byte(`{"status": "success"}`),
	})
	nm := updated.(Model)

	if nm.inFlight {
		t.Fatalf("expected inFlight to clear once a response arrives")
	}
	if len(nm.entries) != 1 || !strings.Contains(nm.entries[0], "action_recipe") {
		t.Fatalf("expected the action_recipe envelope in the feed, got %v", nm.entries)
	}
}
