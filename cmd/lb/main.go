package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/Saman-dev12/lb/internal/config"
	"github.com/Saman-dev12/lb/internal/loadbalancer"
)

func handler(w http.ResponseWriter, r *http.Request) {
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

func register(w http.ResponseWriter, r *http.Request) {
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
		parsedURL, err := url.Parse(cfg.Backends[i].Url)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid URL: " + cfg.Backends[i].Url})
			return
		}
		cfg.Backends[i].Parsed = parsedURL
		cfg.Backends[i].Proxy = httputil.NewSingleHostReverseProxy(parsedURL)
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

func status(w http.ResponseWriter, r *http.Request) {
	loadbalancer.ConfigMu.RLock()
	if loadbalancer.Configuration.Algorithm == "" || len(loadbalancer.Configuration.Backends) == 0 {
		loadbalancer.ConfigMu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "Please register the load balancer first"})
		return
	}

	type backendStatus struct {
		URL        string `json:"url"`
		Health     bool   `json:"health"`
		ActiveConn int64  `json:"activeConn"`
	}

	type statusResponse struct {
		Algorithm        string          `json:"algorithm"`
		HealthCheckRoute string          `json:"healthCheckRoute"`
		Duration         int             `json:"duration"`
		Backends         []backendStatus `json:"backends"`
	}

	resp := statusResponse{
		Algorithm:        loadbalancer.Configuration.Algorithm,
		HealthCheckRoute: loadbalancer.Configuration.HealthCheckRoute,
		Duration:         loadbalancer.Configuration.Duration,
		Backends:         make([]backendStatus, 0, len(loadbalancer.Configuration.Backends)),
	}

	for i := range loadbalancer.Configuration.Backends {
		backend := &loadbalancer.Configuration.Backends[i]
		resp.Backends = append(resp.Backends, backendStatus{
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

func main() {
	r := http.NewServeMux()

	r.HandleFunc("/", handler)
	r.HandleFunc("POST /register", register)
	r.HandleFunc("GET /status", status)
	port := ":8888"
	server := &http.Server{
		Addr:    port,
		Handler: r,
	}
	fmt.Println("Server is listening on port 8888")
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
