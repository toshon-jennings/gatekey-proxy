package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/toshon-jennings/gatekey-proxy/config"
	"github.com/toshon-jennings/gatekey-proxy/ui"
)

type ProxyServer struct {
	port string
}

func NewProxyServer(port string) *ProxyServer {
	return &ProxyServer{port: port}
}

func uiFileSystem() http.FileSystem {
	if st, err := os.Stat("ui"); err == nil && st.IsDir() {
		return http.Dir("ui")
	}
	return http.FS(ui.Assets)
}

func (s *ProxyServer) Start() error {
	// API routes for the Dashboard
	http.HandleFunc("/api/keys", s.handleKeysAPI)
	http.HandleFunc("/api/models", s.handleModelsAPI)

	// Gatekey Proxy Route
	http.HandleFunc("/v1/chat/completions", s.handleChatCompletions)

	// Serve the dashboard from ./ui when it exists (live edits, no rebuild);
	// otherwise fall back to the copy embedded in the binary so
	// "gatekey-proxy start" works from any directory.
	http.Handle("/", http.FileServer(uiFileSystem()))

	// Bind to localhost only for security
	addr := fmt.Sprintf("127.0.0.1:%s", s.port)
	log.Printf("Starting Gatekey Proxy server securely on http://%s", addr)
	return http.ListenAndServe(addr, nil)
}

func (s *ProxyServer) handleModelsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		models, err := config.LoadModels()
		if err != nil {
			http.Error(w, `{"error":"failed to get models"}`, http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(models)

	case http.MethodPut:
		var models []config.ModelSetting
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&models); err != nil {
			http.Error(w, `{"error":"invalid model settings"}`, http.StatusBadRequest)
			return
		}

		for i := range models {
			models[i].Provider = strings.ToLower(strings.TrimSpace(models[i].Provider))
			models[i].Model = strings.TrimSpace(models[i].Model)
		}
		if err := config.SaveModels(models); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(models)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *ProxyServer) handleKeysAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		providers, err := config.GetAllProviders()
		if err != nil {
			http.Error(w, `{"error":"failed to get providers"}`, http.StatusInternalServerError)
			return
		}
		if providers == nil {
			providers = []string{}
		}
		json.NewEncoder(w).Encode(providers)

	case http.MethodPost:
		var req struct {
			Provider string `json:"provider"`
			Key      string `json:"key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if req.Provider == "" || req.Key == "" {
			http.Error(w, `{"error":"provider and key are required"}`, http.StatusBadRequest)
			return
		}
		if err := config.SetKey(req.Provider, req.Key); err != nil {
			http.Error(w, `{"error":"failed to save key"}`, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"success":true}`))

	case http.MethodDelete:
		provider := r.URL.Query().Get("provider")
		if provider == "" {
			http.Error(w, `{"error":"provider query parameter is required"}`, http.StatusBadRequest)
			return
		}
		if err := config.DeleteKey(provider); err != nil {
			http.Error(w, `{"error":"failed to delete key"}`, http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"success":true}`))

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *ProxyServer) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read body to inspect the model
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	r.Body.Close()

	var reqBody map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	modelRaw, ok := reqBody["model"].(string)
	if !ok {
		http.Error(w, "Missing 'model' field", http.StatusBadRequest)
		return
	}

	// Determine provider and target URL
	provider, targetModel, targetURL := s.routeModel(modelRaw)

	// Fetch key securely from local config
	key, err := config.GetKey(provider)
	if err != nil {
		log.Printf("Missing key for provider: %s", provider)
		http.Error(w, fmt.Sprintf("Missing key for provider '%s'. Use CLI to add it.", provider), http.StatusUnauthorized)
		return
	}

	// Update body with the real model name if we stripped the prefix
	reqBody["model"] = targetModel
	newBodyBytes, _ := json.Marshal(reqBody)

	// Create forwarding request
	proxyReq, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(newBodyBytes))
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// Copy headers from original request
	for k, vv := range r.Header {
		for _, v := range vv {
			proxyReq.Header.Add(k, v)
		}
	}

	// Inject security key and correct content type
	proxyReq.Header.Set("Authorization", "Bearer "+key)
	proxyReq.Header.Set("Content-Type", "application/json")
	// Content-Length changed, so reset it
	proxyReq.ContentLength = int64(len(newBodyBytes))

	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("Error forwarding request to %s: %v", targetURL, err)
		http.Error(w, "Failed to forward request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Stream response back to client
	io.Copy(w, resp.Body)
}

// Known provider base URLs. The router strips the prefix and forwards
// to the correct upstream. Providers not listed here but present in the
// config file will use the generic OpenAI-compatible endpoint pattern.
var providerEndpoints = map[string]string{
	"groq":       "https://api.groq.com/openai/v1/chat/completions",
	"openrouter": "https://openrouter.ai/api/v1/chat/completions",
	"openai":     "https://api.openai.com/v1/chat/completions",
	"anthropic":  "https://api.anthropic.com/v1/messages",
	"deepinfra":  "https://api.deepinfra.com/v1/openai/chat/completions",
	"together":   "https://api.together.xyz/v1/chat/completions",
	"opencode":   "https://opencode.ai/zen/v1/chat/completions",
}

func (s *ProxyServer) routeModel(model string) (provider string, targetModel string, url string) {
	// Split on the first "/" to extract the provider prefix.
	// "groq/llama-3.3-70b-versatile" -> provider="groq", targetModel="llama-3.3-70b-versatile"
	// "gpt-4o" (no slash) -> no prefix, defaults to openai
	if idx := strings.Index(model, "/"); idx > 0 {
		provider = model[:idx]
		targetModel = model[idx+1:]

		if endpoint, ok := providerEndpoints[provider]; ok {
			return provider, targetModel, endpoint
		}

		// Provider has a key in the vault but no known endpoint.
		// Assume OpenAI-compatible at https://api.<provider>.com/v1/chat/completions
		return provider, targetModel, fmt.Sprintf("https://api.%s.com/v1/chat/completions", provider)
	}

	// No prefix — pass model through to OpenAI unchanged.
	return "openai", model, "https://api.openai.com/v1/chat/completions"
}
