package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleModelsAPIPersistsNormalizedSettings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := NewProxyServer("8181")

	req := httptest.NewRequest(http.MethodPut, "/api/models", strings.NewReader(`[{"provider":" OpenRouter ","model":" example/new-model "}]`))
	res := httptest.NewRecorder()
	srv.handleModelsAPI(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("PUT /api/models status = %d, body = %s", res.Code, res.Body.String())
	}
	if got := strings.TrimSpace(res.Body.String()); got != `[{"provider":"openrouter","model":"example/new-model"}]` {
		t.Fatalf("PUT /api/models body = %s", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/models", nil)
	res = httptest.NewRecorder()
	srv.handleModelsAPI(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET /api/models status = %d, body = %s", res.Code, res.Body.String())
	}
	if got := strings.TrimSpace(res.Body.String()); got != `[{"provider":"openrouter","model":"example/new-model"}]` {
		t.Fatalf("GET /api/models body = %s", got)
	}
}

func TestHandleModelsAPIRejectsDuplicateSettings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := NewProxyServer("8181")
	req := httptest.NewRequest(http.MethodPut, "/api/models", strings.NewReader(`[
		{"provider":"groq","model":"model-a"},
		{"provider":"GROQ","model":"model-a"}
	]`))
	res := httptest.NewRecorder()

	srv.handleModelsAPI(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("PUT /api/models status = %d, want 400", res.Code)
	}
}

func TestRouteModelStripsKnownAndGenericProviderPrefixes(t *testing.T) {
	srv := NewProxyServer("8181")
	tests := []struct {
		model        string
		wantProvider string
		wantModel    string
		wantURL      string
	}{
		{"openai/gpt-4o", "openai", "gpt-4o", "https://api.openai.com/v1/chat/completions"},
		{"openrouter/anthropic/example", "openrouter", "anthropic/example", "https://openrouter.ai/api/v1/chat/completions"},
		{"future/model-v2", "future", "model-v2", "https://api.future.com/v1/chat/completions"},
		{"gpt-4o", "openai", "gpt-4o", "https://api.openai.com/v1/chat/completions"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			provider, model, url := srv.routeModel(tt.model)
			if provider != tt.wantProvider || model != tt.wantModel || url != tt.wantURL {
				t.Fatalf("routeModel(%q) = (%q, %q, %q), want (%q, %q, %q)", tt.model, provider, model, url, tt.wantProvider, tt.wantModel, tt.wantURL)
			}
		})
	}
}

func TestUpdateSettingsRequireSameOriginDashboardRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := NewProxyServer("8181")
	body := `{"autoCheck":true,"autoInstall":true}`

	req := httptest.NewRequest(http.MethodPut, "/api/update", strings.NewReader(body))
	res := httptest.NewRecorder()
	srv.handleUpdateAPI(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("PUT /api/update without dashboard header = %d, want 403", res.Code)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/update", strings.NewReader(body))
	req.Header.Set("X-Gatekey-Request", "dashboard")
	req.Header.Set("Origin", "http://attacker.example")
	res = httptest.NewRecorder()
	srv.handleUpdateAPI(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("cross-origin PUT /api/update = %d, want 403", res.Code)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/update", strings.NewReader(body))
	req.Header.Set("X-Gatekey-Request", "dashboard")
	req.Header.Set("Origin", "http://example.com")
	res = httptest.NewRecorder()
	srv.handleUpdateAPI(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("same-origin PUT /api/update = %d, body = %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"autoInstall":true`) {
		t.Fatalf("PUT /api/update body = %s", res.Body.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)
	if res.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("Content-Security-Policy header is missing")
	}
	if got := res.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", got)
	}
}
