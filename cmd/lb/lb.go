package main

import (
	"sync/atomic"
)

var rrIndex uint64

func GetNextBackend() *Backend {
	configMu.RLock()
	defer configMu.RUnlock()

	n := uint64(len(Configuration.Backends))

	if n == 0 {
		return nil
	}

	for i := uint64(0); i < n; i++ {
		idx := atomic.AddUint64(&rrIndex, 1) % n

		if Configuration.Backends[idx].Health {
			return &Configuration.Backends[idx]
		}
	}
	return nil
}
