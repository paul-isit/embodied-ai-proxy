package api

import (
	"bytes"
	"embodied-ai-proxy/backend/internal/prompt"
	"embodied-ai-proxy/backend/internal/validator"
	"embodied-ai-proxy/backend/internal/websocket"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func testPipeline(t *testing.T, llmResponseText string) *prompt.Pipeline {
	t.Helper()
	schemaPath, err := filepath.Abs("../../../configs/json_schema.json")
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

	return prompt.New(websocket.NewHub(), v, llmProxy.URL, "Schema:\n{schema_template}\nObjects:{available_objects}\nCommand:{user_command}", []byte(`{}`))
}

func TestPromptHandler_Success(t *testing.T) {
	pipeline := testPipeline(t, `{"status":"success","recipe_name":"t","steps":[{"step_id":1,"action":"home","description":"d","parameters":{}}]}`)

	req := httptest.NewRequest(http.MethodPost, "/api/prompt", bytes.NewBufferString(`{"prompt":"go home","available_objects":["red_cube"]}`))
	w := httptest.NewRecorder()
	PromptHandler(pipeline)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var got prompt.Result
	json.NewDecoder(w.Body).Decode(&got)
	if got.Error != "" || got.Parsed == nil {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestPromptHandler_MissingPrompt(t *testing.T) {
	pipeline := testPipeline(t, `{}`)

	req := httptest.NewRequest(http.MethodPost, "/api/prompt", bytes.NewBufferString(`{}`))
	w := httptest.NewRecorder()
	PromptHandler(pipeline)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestPromptHandler_WrongMethod(t *testing.T) {
	pipeline := testPipeline(t, `{}`)

	req := httptest.NewRequest(http.MethodGet, "/api/prompt", nil)
	w := httptest.NewRecorder()
	PromptHandler(pipeline)(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

func TestPromptHandler_InvalidRecipeStillReturns200WithError(t *testing.T) {
	// Schema validation failures are a valid pipeline outcome, not a transport
	// error - the LLM proxy was reached and answered, so the raw output is
	// present and callers (e.g. the eval runner) need the 200 body to inspect it.
	pipeline := testPipeline(t, `{"status":"success","steps":[]}`) // missing recipe_name

	req := httptest.NewRequest(http.MethodPost, "/api/prompt", bytes.NewBufferString(`{"prompt":"go home"}`))
	w := httptest.NewRecorder()
	PromptHandler(pipeline)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got prompt.Result
	json.NewDecoder(w.Body).Decode(&got)
	if got.Error == "" {
		t.Error("expected schema validation error in result")
	}
}
