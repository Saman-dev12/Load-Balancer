package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/Saman-dev12/lb/internal/config"
	"github.com/Saman-dev12/lb/internal/loadbalancer"
	"github.com/Saman-dev12/lb/internal/proxy"
)

func TestIntegration_LoadBalancerWithCustomProxy(t *testing.T) {
	var count1, count2 atomic.Int64

	// Mock Backend 1
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		count1.Add(1)
		w.Header().Set("X-Backend-ID", "backend-1")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"server":"backend-1"}`))
	}))
	defer backend1.Close()

	// Mock Backend 2
	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		count2.Add(1)
		w.Header().Set("X-Backend-ID", "backend-2")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"server":"backend-2"}`))
	}))
	defer backend2.Close()

	// Parse URLs and configure
	u1, _ := url.Parse(backend1.URL)
	u2, _ := url.Parse(backend2.URL)

	cfg := config.Config{
		Algorithm:        "Round Robin",
		HealthCheckRoute: "/health",
		Duration:         10,
		Backends: []config.Backend{
			{
				Url:    backend1.URL,
				Parsed: u1,
				Proxy:  proxy.NewCustomReverseProxy(u1),
				Health: true,
			},
			{
				Url:    backend2.URL,
				Parsed: u2,
				Proxy:  proxy.NewCustomReverseProxy(u2),
				Health: true,
			},
		},
	}

	loadbalancer.ConfigMu.Lock()
	loadbalancer.Configuration = cfg
	loadbalancer.ConfigMu.Unlock()

	// Setup mux
	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)
	mux.HandleFunc("GET /status", status)

	// Send 4 requests and check round robin distribution
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("request %d failed with status %d", i, res.StatusCode)
		}

		backendID := res.Header.Get("X-Backend-ID")
		if backendID != "backend-1" && backendID != "backend-2" {
			t.Fatalf("unexpected backend ID: %s", backendID)
		}
	}

	if count1.Load() != 2 || count2.Load() != 2 {
		t.Fatalf("expected 2 requests each, got b1=%d, b2=%d", count1.Load(), count2.Load())
	}

	// Verify /status endpoint
	statusReq := httptest.NewRequest(http.MethodGet, "/status", nil)
	statusRec := httptest.NewRecorder()
	mux.ServeHTTP(statusRec, statusReq)

	if statusRec.Code != http.StatusOK {
		t.Fatalf("status code %d, expected 200", statusRec.Code)
	}

	var statusData map[string]interface{}
	if err := json.Unmarshal(statusRec.Body.Bytes(), &statusData); err != nil {
		t.Fatalf("failed to parse status json: %v", err)
	}
	if statusData["algorithm"] != "Round Robin" {
		t.Fatalf("unexpected algorithm in status: %v", statusData["algorithm"])
	}
}

func TestIntegration_RegisterAndForward(t *testing.T) {
	receivedBody := ""
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"received":true}`))
	}))
	defer backend.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)
	mux.HandleFunc("POST /register", register)

	// Register backend
	regPayload := map[string]interface{}{
		"backends": []map[string]string{
			{"url": backend.URL},
		},
		"algorithm": "Round Robin",
	}
	payloadBytes, _ := json.Marshal(regPayload)

	regReq := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(payloadBytes))
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	mux.ServeHTTP(regRec, regReq)

	if regRec.Code != http.StatusOK {
		t.Fatalf("register failed with status %d: %s", regRec.Code, regRec.Body.String())
	}

	// Now send a POST through the load balancer
	postReq := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader([]byte(`{"name":"test"}`)))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	mux.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("forward failed with status %d", postRec.Code)
	}

	if receivedBody != `{"name":"test"}` {
		t.Fatalf("backend did not receive expected body, got: %s", receivedBody)
	}
}
