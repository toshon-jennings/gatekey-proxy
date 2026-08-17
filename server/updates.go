package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/toshon-jennings/gatekey-proxy/config"
)

func (s *ProxyServer) handleUpdateAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode(s.updates.Status())
	case http.MethodPut:
		if !requireDashboardMutation(w, r) {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var preferences config.UpdatePreferences
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&preferences); err != nil {
			writeJSONError(w, "invalid update settings", http.StatusBadRequest)
			return
		}
		status, err := s.updates.SetPreferences(preferences)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(status)
	default:
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *ProxyServer) handleUpdateCheckAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requireDashboardMutation(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	status, err := s.updates.Check(ctx)
	if err != nil {
		writeJSONErrorWithStatus(w, err.Error(), status, http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(status)
}

func (s *ProxyServer) handleUpdateStageAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requireDashboardMutation(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	status, err := s.updates.Stage(ctx)
	if err != nil {
		writeJSONErrorWithStatus(w, err.Error(), status, http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(status)
}

func requireDashboardMutation(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("X-Gatekey-Request") != "dashboard" {
		writeJSONError(w, "dashboard request header required", http.StatusForbidden)
		return false
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		writeJSONError(w, "cross-origin dashboard request rejected", http.StatusForbidden)
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme != "http" || parsed.Host != r.Host {
			writeJSONError(w, "request origin rejected", http.StatusForbidden)
			return false
		}
	}
	return true
}

func writeJSONError(w http.ResponseWriter, message string, status int) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func writeJSONErrorWithStatus(w http.ResponseWriter, message string, updateStatus any, status int) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": message, "status": updateStatus})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}
