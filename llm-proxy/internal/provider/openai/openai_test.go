package openai

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
	var gotReq chatRequest
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotReq)
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []struct {
				Message      message `json:"message"`
				FinishReason string  `json:"finish_reason"`
			}{
				{Message: message{Role: "assistant", Content: " hi there "}, FinishReason: "stop"},
			},
		})
	}))
	defer server.Close()

	adapter := New("gpt-test", server.URL, "secret-key", 1024, 0.1, nil)
	resp, err := adapter.Generate(context.Background(), provider.Request{
		Prompt:            "hello",
		SystemInstruction: "be terse",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "hi there" || resp.FinishReason != "stop" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization header = %q", gotAuth)
	}
	if len(gotReq.Messages) != 2 || gotReq.Messages[0].Role != "system" || gotReq.Messages[1].Content != "hello" {
		t.Errorf("unexpected messages sent: %+v", gotReq.Messages)
	}
}

func TestAdapter_Generate_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	adapter := New("gpt-test", server.URL, "secret-key", 1024, 0.1, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := adapter.Generate(ctx, provider.Request{Prompt: "hi"})
	var timeoutErr *provider.TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("expected *provider.TimeoutError, got %v (%T)", err, err)
	}
}
