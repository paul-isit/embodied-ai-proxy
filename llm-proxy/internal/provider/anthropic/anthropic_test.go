package anthropic

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
	var gotReq messagesRequest
	var gotAPIKey, gotVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		json.NewDecoder(r.Body).Decode(&gotReq)
		json.NewEncoder(w).Encode(messagesResponse{
			Content: []struct {
				Text string `json:"text"`
			}{{Text: " hi there "}},
			StopReason: "end_turn",
		})
	}))
	defer server.Close()

	adapter := New("claude-test", server.URL, "secret-key", 1024, 0.1, nil)
	resp, err := adapter.Generate(context.Background(), provider.Request{
		Prompt:            "hello",
		SystemInstruction: "be terse",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "hi there" || resp.FinishReason != "end_turn" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if gotAPIKey != "secret-key" || gotVersion != apiVersion {
		t.Errorf("unexpected headers: key=%q version=%q", gotAPIKey, gotVersion)
	}
	if gotReq.System != "be terse" || gotReq.Messages[0].Content != "hello" {
		t.Errorf("unexpected request: %+v", gotReq)
	}
}

func TestAdapter_Generate_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	adapter := New("claude-test", server.URL, "secret-key", 1024, 0.1, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := adapter.Generate(ctx, provider.Request{Prompt: "hi"})
	var timeoutErr *provider.TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("expected *provider.TimeoutError, got %v (%T)", err, err)
	}
}
