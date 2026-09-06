package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Request is the provider-agnostic generation request the LLM proxy
// translates into each upstream provider's own API format.
//
// MaxTokens and Temperature are pointers so a per-request override can be
// distinguished from "not set": a plain int/float64 can't tell "the caller
// didn't specify a temperature" apart from "the caller explicitly asked for
// 0" (deterministic decoding), which would otherwise be silently replaced
// by the adapter's configured default. nil means "use the adapter's
// configured default"; a non-nil pointer (including one pointing at 0)
// always wins. See ResolveParams.
type Request struct {
	Prompt            string
	SystemInstruction string
	MaxTokens         *int
	Temperature       *float64
}

// ResolveParams returns the effective max-tokens/temperature for a request:
// the request's override if it set one, otherwise the adapter's configured
// default. Shared by every adapter so the override-vs-default logic (and
// the reasoning in Request's doc comment) lives in exactly one place.
func ResolveParams(defaultMaxTokens int, defaultTemperature float64, req Request) (maxTokens int, temperature float64) {
	maxTokens = defaultMaxTokens
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}
	temperature = defaultTemperature
	if req.Temperature != nil {
		temperature = *req.Temperature
	}
	return maxTokens, temperature
}

// Response is the provider-agnostic result returned to the Go backend.
type Response struct {
	Text         string
	FinishReason string
}

// Provider is implemented by each upstream LLM adapter (Ollama, OpenAI,
// Gemini, Anthropic).
type Provider interface {
	Generate(ctx context.Context, req Request) (Response, error)
}

// TimeoutError wraps an upstream provider timeout so callers can detect it
// with errors.As instead of string-matching.
type TimeoutError struct {
	Provider string
	Cause    error
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("provider %s: request timed out: %v", e.Provider, e.Cause)
}

func (e *TimeoutError) Unwrap() error {
	return e.Cause
}

// StatusError wraps a non-200 response from an upstream provider, with the
// status code exposed so callers can decide what to do with it - e.g.
// whether it's worth a retry (429/5xx) versus a permanent problem (400/401)
// - without string-matching the error text.
type StatusError struct {
	Provider   string
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s: unexpected status %d: %s", e.Provider, e.StatusCode, e.Body)
}

// maxResponseBodyBytes caps how much of an upstream provider's response
// this reads into memory. Generous for any legitimate completion; just
// bounds a misbehaving or compromised upstream from handing back an
// unbounded body.
const maxResponseBodyBytes = 4 << 20 // 4 MiB

// Do executes req and returns its body, handling the request/response
// plumbing every provider adapter otherwise repeated identically: wrapping a
// context-deadline error as a TimeoutError, reading the (size-capped) full
// response body, and rejecting non-200 statuses as a StatusError with the
// (trimmed) response body included. name identifies the provider in error
// messages (e.g. "openai").
func Do(ctx context.Context, client *http.Client, req *http.Request, name string) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, &TimeoutError{Provider: name, Cause: err}
		}
		return nil, fmt.Errorf("%s: request failed: %w", name, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: read response body: %w", name, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{Provider: name, StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	return body, nil
}
