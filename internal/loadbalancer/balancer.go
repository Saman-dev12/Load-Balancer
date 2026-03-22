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
)

func GetNextBackend(req *http.Request) *config.Backend {
	ConfigMu.RLock()
	defer ConfigMu.RUnlock()

	n := uint64(len(Configuration.Backends))

	if n == 0 {
		return nil
	}

	switch Configuration.Algorithm {
	case "Round Robin":
		return getRoundRobinBackend(n)
	case "Random":
		return getRandomBackend(n)
	case "IP Hashing":
		return getIPHashingBackend(req.RemoteAddr, n)
	}

	return nil
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

func returnHealthyBackend(idx int) *config.Backend {
	if Configuration.Backends[idx].Health {
		return &Configuration.Backends[idx]
	}
	return nil
}
