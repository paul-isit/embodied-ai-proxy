package api

import (
	"bytes"
	"context"
	"embodied-ai-proxy/llm-proxy/internal/provider"
	"embodied-ai-proxy/llm-proxy/internal/router"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubProvider struct {
	resp provider.Response
	err  error
}

func (s stubProvider) Generate(ctx context.Context, req provider.Request) (provider.Response, error) {
	return s.resp, s.err
}

func TestGenerateHandler_Success(t *testing.T) {
	rt := router.NewWithProvider(stubProvider{resp: provider.Response{Text: "hi", FinishReason: "stop"}}, 0)
	req := httptest.NewRequest(http.MethodPost, "/generate", bytes.NewBufferString(`{"prompt":"hello"}`))
	w := httptest.NewRecorder()

	GenerateHandler(rt)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got generateResponsePayload
	json.NewDecoder(w.Body).Decode(&got)
	if got.Text != "hi" || got.FinishReason != "stop" {
		t.Errorf("unexpected response: %+v", got)
	}
}

func TestGenerateHandler_MissingPrompt(t *testing.T) {
	rt := router.NewWithProvider(stubProvider{}, 0)
	req := httptest.NewRequest(http.MethodPost, "/generate", bytes.NewBufferString(`{}`))
	w := httptest.NewRecorder()

	GenerateHandler(rt)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestGenerateHandler_TimeoutMapsTo504(t *testing.T) {
	rt := router.NewWithProvider(stubProvider{err: &provider.TimeoutError{Provider: "ollama", Cause: errors.New("deadline exceeded")}}, 0)
	req := httptest.NewRequest(http.MethodPost, "/generate", bytes.NewBufferString(`{"prompt":"hi"}`))
	w := httptest.NewRecorder()

	GenerateHandler(rt)(w, req)

	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", w.Code)
	}
}

func TestGenerateHandler_OtherErrorMapsTo502(t *testing.T) {
	rt := router.NewWithProvider(stubProvider{err: errors.New("boom")}, 0)
	req := httptest.NewRequest(http.MethodPost, "/generate", bytes.NewBufferString(`{"prompt":"hi"}`))
	w := httptest.NewRecorder()

	GenerateHandler(rt)(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
}

// TestGenerateHandler_StatusErrorSanitized guards against raw upstream
// provider error bodies (which may include verbose or vendor-specific
// text) reaching the caller - only a generic, status-code-bearing message
// should cross the /generate boundary.
func TestGenerateHandler_StatusErrorSanitized(t *testing.T) {
	rawBody := "super secret internal upstream diagnostic text"
	rt := router.NewWithProvider(stubProvider{err: &provider.StatusError{
		Provider:   "openai",
		StatusCode: http.StatusBadRequest,
		Body:       rawBody,
	}}, 0)
	req := httptest.NewRequest(http.MethodPost, "/generate", bytes.NewBufferString(`{"prompt":"hi"}`))
	w := httptest.NewRecorder()

	GenerateHandler(rt)(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	var got generateResponsePayload
	json.NewDecoder(w.Body).Decode(&got)
	if strings.Contains(got.Error, rawBody) {
		t.Errorf("response error = %q, must not contain the raw upstream body", got.Error)
	}
	if !strings.Contains(got.Error, "400") {
		t.Errorf("response error = %q, want it to mention the status code", got.Error)
	}
}

func TestGenerateHandler_RequestBodyTooLarge(t *testing.T) {
	rt := router.NewWithProvider(stubProvider{}, 0)
	oversized := `{"prompt":"` + strings.Repeat("a", 2<<20) + `"}` // 2 MiB, over the 1 MiB cap
	req := httptest.NewRequest(http.MethodPost, "/generate", bytes.NewBufferString(oversized))
	w := httptest.NewRecorder()

	GenerateHandler(rt)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an oversized request body", w.Code)
	}
}

func TestGenerateHandler_WrongMethod(t *testing.T) {
	rt := router.NewWithProvider(stubProvider{}, 0)
	req := httptest.NewRequest(http.MethodGet, "/generate", nil)
	w := httptest.NewRecorder()

	GenerateHandler(rt)(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}
