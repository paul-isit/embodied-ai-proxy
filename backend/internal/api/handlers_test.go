package api

import (
	"bytes"
	"embodied-ai-proxy/backend/internal/pipeline"
	"embodied-ai-proxy/backend/internal/validator"
	"embodied-ai-proxy/backend/internal/websocket"
	sharedconfig "embodied-ai-proxy/shared/config"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

	return pipeline.New(websocket.NewHub(), v, llmProxy.URL, "Schema:\n{schema_template}\nObjects:{available_objects}\nCommand:{user_command}", []byte(`{}`))
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
