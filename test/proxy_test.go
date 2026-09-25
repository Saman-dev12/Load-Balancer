package test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Saman-dev12/lb/internal/proxy"
)

func TestProxy_BasicGET(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/hello" {
			t.Errorf("expected path /hello, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("user") != "alice" {
			t.Errorf("expected query user=alice, got %s", r.URL.RawQuery)
		}
		if r.Header.Get("X-Custom-Client") != "test" {
			t.Errorf("expected X-Custom-Client header")
		}

		w.Header().Set("X-Backend-Version", "1.0")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "hello from backend"}`))
	}))
	defer backend.Close()

	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatalf("failed to parse backend URL: %v", err)
	}

	p := proxy.NewCustomReverseProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet, "/hello?user=alice", nil)
	req.Header.Set("X-Custom-Client", "test")
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if res.Header.Get("X-Backend-Version") != "1.0" {
		t.Errorf("expected X-Backend-Version header, got %s", res.Header.Get("X-Backend-Version"))
	}

	body, _ := io.ReadAll(res.Body)
	if string(body) != `{"message": "hello from backend"}` {
		t.Fatalf("unexpected body: %s", string(body))
	}
}

func TestProxy_POSTWithBody(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
		}
		if string(body) != `{"action":"ping"}` {
			t.Errorf("unexpected body: %s", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"action":"pong"}`))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	p := proxy.NewCustomReverseProxy(backendURL)

	req := httptest.NewRequest(http.MethodPost, "/api/ping", strings.NewReader(`{"action":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if string(body) != `{"action":"pong"}` {
		t.Fatalf("unexpected body: %s", string(body))
	}
}

func TestProxy_HopByHopHeadersRemoved(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, h := range []string{"Keep-Alive", "Upgrade", "Proxy-Connection", "X-Custom-Hop"} {
			if r.Header.Get(h) != "" {
				t.Errorf("hop-by-hop header %s was not stripped, got: %s", h, r.Header.Get(h))
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	p := proxy.NewCustomReverseProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Connection", "close, X-Custom-Hop")
	req.Header.Set("Keep-Alive", "timeout=5")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Proxy-Connection", "keep-alive")
	req.Header.Set("X-Custom-Hop", "strip-me")

	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
}

func TestProxy_ForwardingHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "10.0.0.1, 192.168.1.100" {
			t.Errorf("unexpected X-Forwarded-For: %s", xff)
		}
		if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "myapp.example.com" {
			t.Errorf("unexpected X-Forwarded-Host: %s", xfh)
		}
		if xfp := r.Header.Get("X-Forwarded-Proto"); xfp != "http" {
			t.Errorf("unexpected X-Forwarded-Proto: %s", xfp)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	p := proxy.NewCustomReverseProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:54321"
	req.Host = "myapp.example.com"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")

	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
}

func TestProxy_BadGatewayOnBackendDown(t *testing.T) {
	deadURL, _ := url.Parse("http://127.0.0.1:59998")
	p := proxy.NewCustomReverseProxy(deadURL)

	req := httptest.NewRequest(http.MethodGet, "/down", nil)
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 Bad Gateway, got %d", rec.Code)
	}

	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if errResp["error"] != "Bad Gateway" {
		t.Errorf("unexpected error text: %v", errResp)
	}
}
