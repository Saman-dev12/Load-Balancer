package test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Saman-dev12/lb/internal/config"
	"github.com/Saman-dev12/lb/internal/loadbalancer"
)

func setTestConfig(cfg config.Config) {
	loadbalancer.ConfigMu.Lock()
	loadbalancer.Configuration = cfg
	loadbalancer.ConfigMu.Unlock()
}

func TestLoadbalancer_RoundRobin(t *testing.T) {
	cfg := config.Config{
		Algorithm: "Round Robin",
		Backends: []config.Backend{
			{Url: "http://127.0.0.1:9001", Health: true},
			{Url: "http://127.0.0.1:9002", Health: true},
		},
	}
	setTestConfig(cfg)

	seen := make(map[string]int)
	for i := 0; i < 6; i++ {
		lease := loadbalancer.GetNextBackend(httptest.NewRequest(http.MethodGet, "/", nil))
		if lease.Backend == nil {
			t.Fatalf("expected healthy backend")
		}
		seen[lease.Backend.Url]++
		lease.Release()
	}

	if seen["http://127.0.0.1:9001"] != 3 || seen["http://127.0.0.1:9002"] != 3 {
		t.Fatalf("expected 3 requests each, got %v", seen)
	}
}

func TestLoadbalancer_LeastConnections(t *testing.T) {
	cfg := config.Config{
		Algorithm: "Least Connections",
		Backends: []config.Backend{
			{Url: "http://127.0.0.1:9001", Health: true},
			{Url: "http://127.0.0.1:9002", Health: true},
		},
	}
	setTestConfig(cfg)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	lease1 := loadbalancer.GetNextBackend(req)
	if lease1.Backend == nil {
		t.Fatalf("expected backend for lease 1")
	}

	lease2 := loadbalancer.GetNextBackend(req)
	if lease2.Backend == nil {
		t.Fatalf("expected backend for lease 2")
	}

	if lease1.Backend.Url == lease2.Backend.Url {
		t.Fatalf("expected different backends under least connections, got same: %s", lease1.Backend.Url)
	}

	lease1.Release()
	lease2.Release()
}

func TestLoadbalancer_IPHashing(t *testing.T) {
	cfg := config.Config{
		Algorithm: "IP Hashing",
		Backends: []config.Backend{
			{Url: "http://127.0.0.1:9001", Health: true},
			{Url: "http://127.0.0.1:9002", Health: true},
		},
	}
	setTestConfig(cfg)

	reqA := httptest.NewRequest(http.MethodGet, "/", nil)
	reqA.RemoteAddr = "192.168.1.10:12345"

	lease1 := loadbalancer.GetNextBackend(reqA)
	lease2 := loadbalancer.GetNextBackend(reqA)

	if lease1.Backend.Url != lease2.Backend.Url {
		t.Fatalf("expected same backend for identical IP, got %s and %s", lease1.Backend.Url, lease2.Backend.Url)
	}
	lease1.Release()
	lease2.Release()
}

func TestLoadbalancer_SkipsUnhealthy(t *testing.T) {
	cfg := config.Config{
		Algorithm: "Round Robin",
		Backends: []config.Backend{
			{Url: "http://127.0.0.1:9001", Health: false},
			{Url: "http://127.0.0.1:9002", Health: true},
		},
	}
	setTestConfig(cfg)

	for i := 0; i < 4; i++ {
		lease := loadbalancer.GetNextBackend(httptest.NewRequest(http.MethodGet, "/", nil))
		if lease.Backend == nil || lease.Backend.Url != "http://127.0.0.1:9002" {
			t.Fatalf("expected only healthy backend :9002, got %v", lease.Backend)
		}
		lease.Release()
	}
}

func TestLoadbalancer_NoHealthyBackend(t *testing.T) {
	cfg := config.Config{
		Algorithm: "Round Robin",
		Backends: []config.Backend{
			{Url: "http://127.0.0.1:9001", Health: false},
		},
	}
	setTestConfig(cfg)

	lease := loadbalancer.GetNextBackend(httptest.NewRequest(http.MethodGet, "/", nil))
	if lease.Backend != nil {
		t.Fatalf("expected nil backend, got %v", lease.Backend)
	}
	lease.Release()
}
