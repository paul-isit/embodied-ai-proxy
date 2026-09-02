package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"embodied-ai-proxy/llm-proxy/internal/provider"
)

func TestAdapter_Generate_Success(t *testing.T) {
	var gotPath string
	var gotReq generateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		json.NewDecoder(r.Body).Decode(&gotReq)
		json.NewEncoder(w).Encode(generateResponse{
			Candidates: []struct {
				Content      content `json:"content"`
				FinishReason string  `json:"finishReason"`
			}{
				{Content: content{Parts: []part{{Text: " hi there "}}}, FinishReason: "STOP"},
			},
		})
	}))
	defer server.Close()

	adapter := New("gemini-test", server.URL, "secret-key", 1024, 0.1, nil)
	resp, err := adapter.Generate(context.Background(), provider.Request{
		Prompt:            "hello",
		SystemInstruction: "be terse",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "hi there" || resp.FinishReason != "STOP" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if !strings.Contains(gotPath, "gemini-test:generateContent") || !strings.Contains(gotPath, "key=secret-key") {
		t.Errorf("unexpected request path: %q", gotPath)
	}
	if gotReq.SystemInstruction == nil || gotReq.SystemInstruction.Parts[0].Text != "be terse" {
		t.Errorf("system instruction not sent: %+v", gotReq.SystemInstruction)
	}
	if gotReq.Contents[0].Parts[0].Text != "hello" {
		t.Errorf("prompt not sent correctly: %+v", gotReq.Contents)
	}
}

func TestAdapter_Generate_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	adapter := New("gemini-test", server.URL, "secret-key", 1024, 0.1, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := adapter.Generate(ctx, provider.Request{Prompt: "hi"})
	var timeoutErr *provider.TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("expected *provider.TimeoutError, got %v (%T)", err, err)
	}
}
