package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"embodied-ai-proxy/llm-proxy/internal/provider"
)

func TestAdapter_Generate_Success(t *testing.T) {
	var gotReq generateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		json.NewEncoder(w).Encode(generateResponse{
			Response:   "  hello there  ",
			Done:       true,
			DoneReason: "stop",
		})
	}))
	defer server.Close()

	adapter := New("gemma3:1b", server.URL, 1024, 0.1, nil)

	resp, err := adapter.Generate(context.Background(), provider.Request{
		Prompt:            "hi",
		SystemInstruction: "be nice",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "hello there" {
		t.Errorf("Text = %q, want %q", resp.Text, "hello there")
	}
	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "stop")
	}
	if gotReq.Model != "gemma3:1b" || gotReq.Prompt != "hi" || gotReq.System != "be nice" {
		t.Errorf("unexpected request sent: %+v", gotReq)
	}
	if gotReq.Options.NumPredict != 1024 || gotReq.Options.Temperature != 0.1 {
		t.Errorf("unexpected options sent: %+v", gotReq.Options)
	}
}

func TestAdapter_Generate_RequestOverridesDefaults(t *testing.T) {
	var gotReq generateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotReq)
		json.NewEncoder(w).Encode(generateResponse{Response: "ok", Done: true})
	}))
	defer server.Close()

	adapter := New("gemma3:1b", server.URL, 1024, 0.1, nil)
	maxTokens, temperature := 256, 0.7
	_, err := adapter.Generate(context.Background(), provider.Request{
		Prompt:      "hi",
		MaxTokens:   &maxTokens,
		Temperature: &temperature,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if gotReq.Options.NumPredict != 256 || gotReq.Options.Temperature != 0.7 {
		t.Errorf("request-level overrides not applied: %+v", gotReq.Options)
	}
}

// TestAdapter_Generate_ExplicitZeroTemperatureOverridesDefault guards
// against the zero-value bug this used to have: since Request.Temperature
// is a *float64, an explicit 0 (deterministic decoding) must be
// distinguishable from "the caller didn't set an override at all", not
// silently replaced by the adapter's configured default.
func TestAdapter_Generate_ExplicitZeroTemperatureOverridesDefault(t *testing.T) {
	var gotReq generateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotReq)
		json.NewEncoder(w).Encode(generateResponse{Response: "ok", Done: true})
	}))
	defer server.Close()

	adapter := New("gemma3:1b", server.URL, 1024, 0.9, nil) // configured default temperature is 0.9
	zero := 0.0
	_, err := adapter.Generate(context.Background(), provider.Request{Prompt: "hi", Temperature: &zero})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if gotReq.Options.Temperature != 0 {
		t.Errorf("Options.Temperature = %v, want 0 (explicit override must win over the 0.9 default)", gotReq.Options.Temperature)
	}
}

// TestAdapter_Generate_NoOverride_UsesConfiguredDefaults confirms nil
// MaxTokens/Temperature (no per-request override at all) still falls back
// to the adapter's configured defaults, as before.
func TestAdapter_Generate_NoOverride_UsesConfiguredDefaults(t *testing.T) {
	var gotReq generateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotReq)
		json.NewEncoder(w).Encode(generateResponse{Response: "ok", Done: true})
	}))
	defer server.Close()

	adapter := New("gemma3:1b", server.URL, 1024, 0.1, nil)
	_, err := adapter.Generate(context.Background(), provider.Request{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if gotReq.Options.NumPredict != 1024 || gotReq.Options.Temperature != 0.1 {
		t.Errorf("expected configured defaults, got: %+v", gotReq.Options)
	}
}

func TestAdapter_Generate_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	defer server.Close()

	adapter := New("gemma3:1b", server.URL, 1024, 0.1, nil)
	_, err := adapter.Generate(context.Background(), provider.Request{Prompt: "hi"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAdapter_Generate_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		json.NewEncoder(w).Encode(generateResponse{Response: "too slow"})
	}))
	defer server.Close()

	adapter := New("gemma3:1b", server.URL, 1024, 0.1, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := adapter.Generate(ctx, provider.Request{Prompt: "hi"})
	var timeoutErr *provider.TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("expected *provider.TimeoutError, got %v (%T)", err, err)
	}
}
