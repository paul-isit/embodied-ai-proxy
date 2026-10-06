package api

import (
	"bytes"
	"context"
	"embodied-ai-proxy/backend/internal/pipeline"
	"embodied-ai-proxy/backend/internal/rosbridge"
	"embodied-ai-proxy/backend/internal/validator"
	"embodied-ai-proxy/backend/internal/websocket"
	sharedconfig "embodied-ai-proxy/shared/config"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func testPipeline(t *testing.T, llmResponseText string) *pipeline.Pipeline {
	t.Helper()
	schemaPath, err := filepath.Abs("../../../data/config/json_schema.json")
	if err != nil {
		t.Fatalf("resolve schema path: %v", err)
	}
	schemaRaw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema file: %v", err)
	}
	v, err := validator.New(schemaPath, schemaRaw)
	if err != nil {
		t.Fatalf("validator.New() error = %v", err)
	}

	llmProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"text": llmResponseText})
	}))
	t.Cleanup(llmProxy.Close)

	return pipeline.New(websocket.NewHub(), nil, v, llmProxy.URL, "Schema:\n{schema_template}\nObjects:{available_objects}\nCommand:{user_command}", []byte(`{}`))
}

func TestInfoHandler_ReportsServerProxyAndHubState(t *testing.T) {
	p := testPipeline(t, `{}`)
	hub := websocket.NewHub()

	cfg := &sharedconfig.AppConfig{
		Server: sharedconfig.ServerConfig{Port: 8080, ProxyURL: "http://localhost:8081"},
		Proxy: sharedconfig.ProxyConfig{
			Port: 8081,
			LLMConfig: sharedconfig.LLMConfig{
				Provider: "ollama",
				Model:    "gemma3:1b",
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/info", nil)
	w := httptest.NewRecorder()
	InfoHandler(cfg, hub, p)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var got infoResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Server.Port != 8080 || got.LLM.Provider != "ollama" || got.LLM.Model != "gemma3:1b" {
		t.Errorf("unexpected response: %+v", got)
	}
	if got.BridgeConnected {
		t.Error("expected bridge_connected=false with no bridge dialed")
	}
	if got.SystemPrompt == "" {
		t.Error("expected system_prompt to be populated")
	}
}

type fakeResetter struct {
	message string
	err     error
}

func (f *fakeResetter) ResetEnvironment(ctx context.Context) (string, error) {
	return f.message, f.err
}

func TestResetHandler_Success(t *testing.T) {
	rb := &fakeResetter{message: "Environment reset: 4 object(s), 2 obstacle(s) restored to configured defaults"}

	req := httptest.NewRequest(http.MethodPost, "/api/reset", nil)
	w := httptest.NewRecorder()
	ResetHandler(rb)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var got resetResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.Success || got.Message != rb.message {
		t.Errorf("unexpected response: %+v", got)
	}
}

func TestResetHandler_Failure(t *testing.T) {
	rb := &fakeResetter{err: errors.New("environment reset failed: Failed to apply the reset planning scene")}

	req := httptest.NewRequest(http.MethodPost, "/api/reset", nil)
	w := httptest.NewRecorder()
	ResetHandler(rb)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (errors are reported in the body, not the status)", w.Code)
	}

	var got resetResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Success || got.Message != rb.err.Error() {
		t.Errorf("unexpected response: %+v", got)
	}
}

func TestResetHandler_WrongMethod(t *testing.T) {
	rb := &fakeResetter{}

	req := httptest.NewRequest(http.MethodGet, "/api/reset", nil)
	w := httptest.NewRecorder()
	ResetHandler(rb)(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

func TestPromptHandler_Success(t *testing.T) {
	p := testPipeline(t, `{"status":"success","recipe_name":"t","steps":[{"step_id":1,"action":"home","description":"d","parameters":{}}]}`)

	req := httptest.NewRequest(http.MethodPost, "/api/prompt", bytes.NewBufferString(`{"prompt":"go home","available_objects":["red_cube"]}`))
	w := httptest.NewRecorder()
	PromptHandler(p)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var got pipeline.Result
	json.NewDecoder(w.Body).Decode(&got)
	if got.Error != "" || got.Parsed == nil {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestPromptHandler_MissingPrompt(t *testing.T) {
	p := testPipeline(t, `{}`)

	req := httptest.NewRequest(http.MethodPost, "/api/prompt", bytes.NewBufferString(`{}`))
	w := httptest.NewRecorder()
	PromptHandler(p)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestPromptHandler_WrongMethod(t *testing.T) {
	p := testPipeline(t, `{}`)

	req := httptest.NewRequest(http.MethodGet, "/api/prompt", nil)
	w := httptest.NewRecorder()
	PromptHandler(p)(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

func TestPromptHandler_InvalidRecipeStillReturns200WithError(t *testing.T) {
	p := testPipeline(t, `{"status":"success","steps":[]}`) // missing recipe_name

	req := httptest.NewRequest(http.MethodPost, "/api/prompt", bytes.NewBufferString(`{"prompt":"go home"}`))
	w := httptest.NewRecorder()
	PromptHandler(p)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got pipeline.Result
	json.NewDecoder(w.Body).Decode(&got)
	if got.Error == "" {
		t.Error("expected schema validation error in result")
	}
}

type fakeScanner struct {
	refreshes [][]string // object names returned by each refresh, in order
	message   string
	err       error
}

func (f *fakeScanner) RefreshEnvironmentParams(ctx context.Context) (rosbridge.EnvironmentParams, error) {
	objects := f.refreshes[0]
	f.refreshes = f.refreshes[1:]
	return rosbridge.EnvironmentParams{Objects: objects}, nil
}

func (f *fakeScanner) Snapshot(ctx context.Context) (string, error) {
	return f.message, f.err
}

func scan(t *testing.T, rb scanner) scanResponse {
	t.Helper()
	w := httptest.NewRecorder()
	ScanHandler(rb)(w, httptest.NewRequest(http.MethodPost, "/api/scan", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got scanResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return got
}

func TestScanHandler_ReportsAddedAndRemoved(t *testing.T) {
	rb := &fakeScanner{
		refreshes: [][]string{{"blue_cube", "red_bottle"}, {"blue_cube", "green_cup", "red_object"}},
		message:   "Scene updated: 2 added, 1 updated, 1 removed, 3 total",
	}

	got := scan(t, rb)

	if !got.Success || got.Message != rb.message {
		t.Fatalf("unexpected response: %+v", got)
	}
	if !slices.Equal(got.Added, []string{"green_cup", "red_object"}) || !slices.Equal(got.Removed, []string{"red_bottle"}) || got.Unchanged != 1 {
		t.Errorf("added %v, removed %v, unchanged %d; want [green_cup red_object], [red_bottle], 1", got.Added, got.Removed, got.Unchanged)
	}
}

func TestScanHandler_SnapshotFailure(t *testing.T) {
	rb := &fakeScanner{refreshes: [][]string{{"blue_cube"}}, err: errors.New("call /vision/snapshot (is the vision node running?): service does not exist")}

	got := scan(t, rb)

	if got.Success || got.Message != rb.err.Error() {
		t.Errorf("unexpected response: %+v", got)
	}
}

func TestScanHandler_WrongMethod(t *testing.T) {
	w := httptest.NewRecorder()
	ScanHandler(&fakeScanner{})(w, httptest.NewRequest(http.MethodGet, "/api/scan", nil))

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}
