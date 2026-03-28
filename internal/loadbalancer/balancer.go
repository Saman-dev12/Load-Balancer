package loadbalancer

import (
	"hash/fnv"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/Saman-dev12/lb/internal/config"
)

var (
	Configuration config.Config
	ConfigMu      sync.RWMutex
	HealthOnce    sync.Once
	rrIndex       uint64
	LeastMu       sync.Mutex
	strategies    = map[string]BackendSelectionStrategy{
		"Round Robin":       roundRobinStrategy,
		"Random":            randomStrategy,
		"IP Hashing":        ipHashingStrategy,
		"Least Connections": leastConnectionsStrategy,
	}
)

type BackendSelectionStrategy func(req *http.Request, n uint64) *BackendLease

type BackendLease struct {
	Backend *config.Backend
	release func()
}

func (l *BackendLease) Release() {
	if l == nil {
		return
	}
	if l.release != nil {
		l.release()
		l.release = nil
	}
}

func GetNextBackend(req *http.Request) *BackendLease {
	ConfigMu.RLock()
	defer ConfigMu.RUnlock()

	n := uint64(len(Configuration.Backends))

	if n == 0 {
		return &BackendLease{}
	}

	strategy, ok := strategies[Configuration.Algorithm]
	if !ok {
		strategy = roundRobinStrategy
	}

	return strategy(req, n)
}

func roundRobinStrategy(_ *http.Request, n uint64) *BackendLease {
	return &BackendLease{Backend: getRoundRobinBackend(n)}
}

func randomStrategy(_ *http.Request, n uint64) *BackendLease {
	return &BackendLease{Backend: getRandomBackend(n)}
}

func ipHashingStrategy(req *http.Request, n uint64) *BackendLease {
	if req == nil {
		return &BackendLease{Backend: getRoundRobinBackend(n)}
	}
	return &BackendLease{Backend: getIPHashingBackend(req.RemoteAddr, n)}
}

func leastConnectionsStrategy(_ *http.Request, n uint64) *BackendLease {
	LeastMu.Lock()
	defer LeastMu.Unlock()

	backend := leastConnectionBackend(n)
	if backend != nil {
		backend.ActiveConn.Add(1)
		return &BackendLease{
			Backend: backend,
			release: func() { backend.ActiveConn.Add(-1) },
		}
	}

	return &BackendLease{}
}

func getRandomBackend(n uint64) *config.Backend {
	start := rand.Intn(int(n))
	for i := 0; i < int(n); i++ {
		idx := (start + i) % int(n)
		if backend := returnHealthyBackend(idx); backend != nil {
			return backend
		}
	}
	return nil
}

func getRoundRobinBackend(n uint64) *config.Backend {
	for i := uint64(0); i < n; i++ {
		idx := atomic.AddUint64(&rrIndex, 1) % n
		if backend := returnHealthyBackend(int(idx)); backend != nil {
			return backend
		}
	}
	return nil
}

func getIPHashingBackend(clientIP string, n uint64) *config.Backend {
	h := fnv.New32a()
	h.Write([]byte(clientIP))
	hashValue := h.Sum32()

	start := int(hashValue) % int(n)
	for i := 0; i < int(n); i++ {
		idx := (start + i) % int(n)
		if backend := returnHealthyBackend(idx); backend != nil {
			return backend
		}
	}
	return nil
}

func leastConnectionBackend(n uint64) *config.Backend {
	var bestBackend *config.Backend
	var minConn int64 = -1
	start := int(atomic.AddUint64(&rrIndex, 1) % n)

	for j := 0; j < int(n); j++ {
		i := (start + j) % int(n)
		// Only consider healthy backends
		if !Configuration.Backends[i].Health {
			continue
		}

		conns := Configuration.Backends[i].ActiveConn.Load()
		if minConn == -1 || conns < minConn {
			minConn = conns
			bestBackend = &Configuration.Backends[i]
		}
	}
	return bestBackend
}

func returnHealthyBackend(idx int) *config.Backend {
	if Configuration.Backends[idx].Health {
		return &Configuration.Backends[idx]
	}
	return nil
}
