package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/Saman-dev12/lb/internal/config"
	"github.com/Saman-dev12/lb/internal/loadbalancer"
	"github.com/Saman-dev12/lb/internal/proxy"
)

type BackendStatus struct {
	URL        string `json:"url"`
	Health     bool   `json:"health"`
	ActiveConn int64  `json:"activeConn"`
}

type StatusResponse struct {
	Algorithm        string          `json:"algorithm"`
	HealthCheckRoute string          `json:"healthCheckRoute"`
	Duration         int             `json:"duration"`
	Backends         []BackendStatus `json:"backends"`
}

func NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", Handler)
	mux.HandleFunc("POST /register", Register)
	mux.HandleFunc("GET /status", Status)
	mux.HandleFunc("GET /healthz", Healthz)
	return mux
}

func NewServer(addr string) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: NewMux(),
	}
}

func Handler(w http.ResponseWriter, r *http.Request) {
	lease := loadbalancer.GetNextBackend(r)
	defer lease.Release()
	backend := lease.Backend

	if backend == nil || backend.Proxy == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "No healthy backend available"})
		return
	}

	backend.Proxy.ServeHTTP(w, r)
}

func Register(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	defer r.Body.Close()

	var cfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if len(cfg.Backends) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "at least one URL is required"})
		return
	}

	cfg.CheckAndCorrectConfig()

	for i := range cfg.Backends {
		rawURL := cfg.Backends[i].Url
		if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
			rawURL = "http://" + rawURL
			cfg.Backends[i].Url = rawURL
		}

		parsedURL, err := url.Parse(rawURL)
		if err != nil || parsedURL.Host == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid URL: " + cfg.Backends[i].Url})
			return
		}
		cfg.Backends[i].Parsed = parsedURL
		cfg.Backends[i].Proxy = proxy.NewCustomReverseProxy(parsedURL)
		cfg.Backends[i].Health = true
	}

	loadbalancer.ConfigMu.Lock()
	loadbalancer.Configuration = cfg
	loadbalancer.ConfigMu.Unlock()

	loadbalancer.HealthOnce.Do(func() {
		go loadbalancer.HealthCheck()
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Configuration registered successfully"})
}

func Status(w http.ResponseWriter, r *http.Request) {
	loadbalancer.ConfigMu.RLock()
	if loadbalancer.Configuration.Algorithm == "" || len(loadbalancer.Configuration.Backends) == 0 {
		loadbalancer.ConfigMu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "Please register the load balancer first"})
		return
	}

	resp := StatusResponse{
		Algorithm:        loadbalancer.Configuration.Algorithm,
		HealthCheckRoute: loadbalancer.Configuration.HealthCheckRoute,
		Duration:         loadbalancer.Configuration.Duration,
		Backends:         make([]BackendStatus, 0, len(loadbalancer.Configuration.Backends)),
	}

	for i := range loadbalancer.Configuration.Backends {
		backend := &loadbalancer.Configuration.Backends[i]
		resp.Backends = append(resp.Backends, BackendStatus{
			URL:        backend.Url,
			Health:     backend.Health,
			ActiveConn: backend.ActiveConn.Load(),
		})
	}
	loadbalancer.ConfigMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
