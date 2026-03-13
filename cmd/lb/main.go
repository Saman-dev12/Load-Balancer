package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

type Backend struct {
	Url    string
	Health bool
}

type Config struct {
	Backends            []Backend
	HealthCheckRoute    string
	Duration            int
	MinRequestThreshold int
	MaxRequestThreshold int
	Algorithm           string
}

func (c *Config) CheckAndCorrectConfig() {
	if c.HealthCheckRoute == "" {
		c.HealthCheckRoute = "/health"
	}

	if c.Duration == 0 {
		c.Duration = 300
	}

	if c.MinRequestThreshold == 0 {
		c.MinRequestThreshold = 1
	}

	if c.MaxRequestThreshold == 0 {
		c.MaxRequestThreshold = 5
	}

	if c.Algorithm == "" {
		c.Algorithm = "Round Robin"
	}

}

var (
	Configuration   Config
	configMu        sync.RWMutex
	healthCheckOnce sync.Once
)

func handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	backend := GetNextBackend()

	if backend == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "No backend found"})
		return
	}

	target, err := url.Parse(backend.Url)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid URL"})
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, r)
}

func register(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	defer r.Body.Close()

	var cfg Config
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

	configMu.Lock()
	Configuration = cfg
	configMu.Unlock()

	healthCheckOnce.Do(func() {
		go HealthCheck()
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Configuration registered successfully"})
}

func main() {
	r := http.NewServeMux()

	r.HandleFunc("/", handler)
	r.HandleFunc("/register", register)

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
