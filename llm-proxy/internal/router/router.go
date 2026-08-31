package router

import (
	"context"
	"embodied-ai-proxy/llm-proxy/internal/provider"
	"embodied-ai-proxy/llm-proxy/internal/provider/anthropic"
	"embodied-ai-proxy/llm-proxy/internal/provider/gemini"
	"embodied-ai-proxy/llm-proxy/internal/provider/ollama"
	"embodied-ai-proxy/llm-proxy/internal/provider/openai"
	sharedconfig "embodied-ai-proxy/shared/config"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	// maxAttempts is 1 retry (2 attempts total) - enough to ride out a
	// single transient blip without turning a slow provider into a slow
	// retry storm.
	maxAttempts  = 2
	retryBackoff = 250 * time.Millisecond
)

// Router dispatches generation requests to whichever provider is configured
// and applies the configured request timeout uniformly across providers.
type Router struct {
	provider provider.Provider
	timeout  time.Duration
}

// New builds a Router for the provider named in cfg.Provider.
func New(cfg sharedconfig.LLMConfig, httpClient *http.Client) (*Router, error) {
	p, err := build(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	return NewWithProvider(p, time.Duration(cfg.TimeoutSeconds)*time.Second), nil
}

// NewWithProvider builds a Router around an already-constructed provider,
// primarily so callers (tests, alternate wiring) can inject a stub.
func NewWithProvider(p provider.Provider, timeout time.Duration) *Router {
	return &Router{provider: p, timeout: timeout}
}

func build(cfg sharedconfig.LLMConfig, httpClient *http.Client) (provider.Provider, error) {
	switch cfg.Provider {
	case "ollama":
		return ollama.New(cfg.Model, cfg.BaseURL, cfg.MaxTokens, cfg.Temperature, httpClient), nil
	case "openai":
		return openai.New(cfg.Model, cfg.BaseURL, cfg.APIKey, cfg.MaxTokens, cfg.Temperature, httpClient), nil
	case "gemini":
		return gemini.New(cfg.Model, cfg.BaseURL, cfg.APIKey, cfg.MaxTokens, cfg.Temperature, httpClient), nil
	case "anthropic":
		return anthropic.New(cfg.Model, cfg.BaseURL, cfg.APIKey, cfg.MaxTokens, cfg.Temperature, httpClient), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider %q", cfg.Provider)
	}
}

// Generate dispatches req to the configured provider, bounding it with the
// router's configured timeout (if any) regardless of which provider is
// active, retrying once if the failure looks transient (see retryable).
func (r *Router) Generate(ctx context.Context, req provider.Request) (provider.Response, error) {
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := r.provider.Generate(ctx, req)
		if err == nil {
			if attempt > 1 {
				log.Printf("[LLMProxy] router: attempt %d/%d succeeded after %d earlier failure(s)", attempt, maxAttempts, attempt-1)
			}
			return resp, nil
		}
		lastErr = err
		if attempt == maxAttempts || !retryable(err) {
			log.Printf("[LLMProxy] router: attempt %d/%d failed, giving up: %v", attempt, maxAttempts, err)
			break
		}
		log.Printf("[LLMProxy] router: attempt %d/%d failed, retrying in %s: %v", attempt, maxAttempts, retryBackoff, err)
		select {
		case <-time.After(retryBackoff):
		case <-ctx.Done():
			log.Printf("[LLMProxy] router: giving up, context done while waiting to retry: %v", ctx.Err())
			return provider.Response{}, err
		}
	}
	return provider.Response{}, lastErr
}

// retryable reports whether err looks like a transient failure worth one
// retry. A timeout is deliberately excluded: it already consumed its share
// of the deadline, so retrying immediately is more likely to just time out
// again (or blow through what's left of the caller's own budget) than help.
// A non-2xx status is only retried for 429 (rate limited) or 5xx (upstream
// server error) - anything else (400, 401, ...) is a permanent problem that
// retrying can't fix. Any other error (connection refused/reset, DNS
// failure, etc.) is assumed transient and gets one retry.
func retryable(err error) bool {
	var timeoutErr *provider.TimeoutError
	if errors.As(err, &timeoutErr) {
		return false
	}
	var statusErr *provider.StatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusTooManyRequests || statusErr.StatusCode >= 500
	}
	return true
}
