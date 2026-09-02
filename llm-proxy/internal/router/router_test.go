package router

import (
	"context"
	"embodied-ai-proxy/llm-proxy/internal/provider"
	sharedconfig "embodied-ai-proxy/shared/config"
	"errors"
	"net/http"
	"testing"
	"time"
)

type stubProvider struct {
	gotCtx context.Context
	resp   provider.Response
	err    error
}

func (s *stubProvider) Generate(ctx context.Context, req provider.Request) (provider.Response, error) {
	s.gotCtx = ctx
	return s.resp, s.err
}

func TestBuild_UnknownProvider(t *testing.T) {
	_, err := build(sharedconfig.LLMConfig{Provider: "not-a-real-provider"}, nil)
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

func TestBuild_KnownProviders(t *testing.T) {
	for _, name := range []string{"ollama", "openai", "gemini", "anthropic"} {
		if _, err := build(sharedconfig.LLMConfig{Provider: name}, nil); err != nil {
			t.Errorf("build(%q) unexpected error: %v", name, err)
		}
	}
}

func TestRouter_Generate_AppliesTimeout(t *testing.T) {
	stub := &stubProvider{resp: provider.Response{Text: "ok"}}
	r := NewWithProvider(stub, 5*time.Second)

	if _, err := r.Generate(context.Background(), provider.Request{Prompt: "hi"}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if _, ok := stub.gotCtx.Deadline(); !ok {
		t.Error("expected provider to receive a context with a deadline")
	}
}

func TestRouter_Generate_NoTimeoutConfigured(t *testing.T) {
	stub := &stubProvider{resp: provider.Response{Text: "ok"}}
	r := NewWithProvider(stub, 0)

	if _, err := r.Generate(context.Background(), provider.Request{Prompt: "hi"}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if _, ok := stub.gotCtx.Deadline(); ok {
		t.Error("expected no deadline when timeout is 0")
	}
}

func TestRouter_Generate_PropagatesProviderError(t *testing.T) {
	wantErr := errors.New("boom")
	stub := &stubProvider{err: wantErr}
	r := NewWithProvider(stub, 0)

	_, err := r.Generate(context.Background(), provider.Request{Prompt: "hi"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Generate() error = %v, want %v", err, wantErr)
	}
}

// sequenceProvider returns one scripted (response, error) pair per call,
// repeating the last entry if called more times than scripted, so retry
// behavior can be exercised deterministically.
type sequenceProvider struct {
	calls   int
	results []struct {
		resp provider.Response
		err  error
	}
}

func (s *sequenceProvider) Generate(ctx context.Context, req provider.Request) (provider.Response, error) {
	i := s.calls
	if i >= len(s.results) {
		i = len(s.results) - 1
	}
	s.calls++
	return s.results[i].resp, s.results[i].err
}

func TestRouter_Generate_RetriesOnceOnRetryableStatusError(t *testing.T) {
	stub := &sequenceProvider{results: []struct {
		resp provider.Response
		err  error
	}{
		{err: &provider.StatusError{Provider: "ollama", StatusCode: http.StatusServiceUnavailable}},
		{resp: provider.Response{Text: "recovered"}},
	}}
	r := NewWithProvider(stub, 0)

	resp, err := r.Generate(context.Background(), provider.Request{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Generate() error = %v, want nil after retry succeeds", err)
	}
	if resp.Text != "recovered" || stub.calls != 2 {
		t.Errorf("resp = %+v, calls = %d, want text=recovered, calls=2", resp, stub.calls)
	}
}

func TestRouter_Generate_DoesNotRetryNonRetryableStatusError(t *testing.T) {
	stub := &sequenceProvider{results: []struct {
		resp provider.Response
		err  error
	}{
		{err: &provider.StatusError{Provider: "openai", StatusCode: http.StatusBadRequest}},
		{resp: provider.Response{Text: "should never be reached"}},
	}}
	r := NewWithProvider(stub, 0)

	_, err := r.Generate(context.Background(), provider.Request{Prompt: "hi"})
	if err == nil {
		t.Fatal("expected error for a 400 status, got nil")
	}
	if stub.calls != 1 {
		t.Errorf("calls = %d, want 1 (a 400 is not retryable)", stub.calls)
	}
}

func TestRouter_Generate_DoesNotRetryTimeout(t *testing.T) {
	stub := &sequenceProvider{results: []struct {
		resp provider.Response
		err  error
	}{
		{err: &provider.TimeoutError{Provider: "gemini", Cause: context.DeadlineExceeded}},
		{resp: provider.Response{Text: "should never be reached"}},
	}}
	r := NewWithProvider(stub, 0)

	_, err := r.Generate(context.Background(), provider.Request{Prompt: "hi"})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if stub.calls != 1 {
		t.Errorf("calls = %d, want 1 (a timeout must not be retried)", stub.calls)
	}
}

func TestRouter_Generate_RetriesGenericTransportError(t *testing.T) {
	stub := &sequenceProvider{results: []struct {
		resp provider.Response
		err  error
	}{
		{err: errors.New("connection refused")},
		{resp: provider.Response{Text: "recovered"}},
	}}
	r := NewWithProvider(stub, 0)

	resp, err := r.Generate(context.Background(), provider.Request{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Generate() error = %v, want nil after retry succeeds", err)
	}
	if resp.Text != "recovered" || stub.calls != 2 {
		t.Errorf("resp = %+v, calls = %d, want text=recovered, calls=2", resp, stub.calls)
	}
}
