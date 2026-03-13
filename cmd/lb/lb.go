package main

import (
	"sync"
)

var (
	currentIndex int
	lbMu         sync.Mutex
)

func GetNextBackend() *Backend {
	configMu.Lock()
	defer configMu.Unlock()

	lbMu.Lock()
	defer lbMu.Unlock()

	n := len(Configuration.Backends)

	for i := 0; i < n; i++ {
		idx := (currentIndex + i) % n

		if Configuration.Backends[idx].Health {
			currentIndex = idx + 1
			return &Configuration.Backends[idx]
		}
	}
	return nil
}
