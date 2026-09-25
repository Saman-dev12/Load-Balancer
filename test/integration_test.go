package test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Saman-dev12/lb/internal/server"
)

func TestIntegration_RegisterAndForward(t *testing.T) {
	var count1, count2 atomic.Int64

	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count1.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"server":"b1"}`))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count2.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"server":"b2"}`))
	}))
	defer backend2.Close()

	mux := server.NewMux()

	regPayload := map[string]interface{}{
		"backends": []map[string]string{
			{"url": backend1.URL},
			{"url": backend2.URL},
		},
		"algorithm": "Round Robin",
	}
	payloadBytes, _ := json.Marshal(regPayload)

	regReq := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(payloadBytes))
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	mux.ServeHTTP(regRec, regReq)

	if regRec.Code != http.StatusOK {
		t.Fatalf("register failed: %d, body: %s", regRec.Code, regRec.Body.String())
	}

	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("forwarding failed with status %d", rec.Code)
		}
	}

	if count1.Load() != 2 || count2.Load() != 2 {
		t.Fatalf("expected round robin 2 each, got b1=%d, b2=%d", count1.Load(), count2.Load())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/status", nil)
	statusRec := httptest.NewRecorder()
	mux.ServeHTTP(statusRec, statusReq)

	if statusRec.Code != http.StatusOK {
		t.Fatalf("status failed: %d", statusRec.Code)
	}

	var statusResp server.StatusResponse
	if err := json.Unmarshal(statusRec.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("failed to decode status: %v", err)
	}
	if len(statusResp.Backends) != 2 {
		t.Fatalf("expected 2 backends in status, got %d", len(statusResp.Backends))
	}

	healthzReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthzRec := httptest.NewRecorder()
	mux.ServeHTTP(healthzRec, healthzReq)

	if healthzRec.Code != http.StatusOK {
		t.Fatalf("healthz failed: %d", healthzRec.Code)
	}
}

func TestIntegration_PostPayloadForwarding(t *testing.T) {
	var receivedBody string

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"received":true}`))
	}))
	defer backend.Close()

	mux := server.NewMux()

	regPayload := map[string]interface{}{
		"backends": []map[string]string{
			{"url": backend.URL},
		},
		"algorithm": "Round Robin",
	}
	payloadBytes, _ := json.Marshal(regPayload)

	regReq := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(payloadBytes))
	regRec := httptest.NewRecorder()
	mux.ServeHTTP(regRec, regReq)

	postReq := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader([]byte(`{"item":"test"}`)))
	postRec := httptest.NewRecorder()
	mux.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", postRec.Code)
	}

	if receivedBody != `{"item":"test"}` {
		t.Fatalf("expected body %q, got %q", `{"item":"test"}`, receivedBody)
	}
}
