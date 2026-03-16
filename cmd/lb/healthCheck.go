package main

import (
	"fmt"
	"net/http"
	"time"
)

func HealthCheck() {
	ticker := time.NewTicker(time.Duration(Configuration.Duration) * time.Second)
	defer ticker.Stop()

	client := &http.Client{Timeout: 2 * time.Second}

	for range ticker.C {
		configMu.RLock()
		type checkData struct {
			idx    int
			urlStr string
			chkUrl string
		}
		var checks []checkData
		route := Configuration.HealthCheckRoute

		for i, b := range Configuration.Backends {
			if b.parsed != nil {
				checks = append(checks, checkData{
					idx:    i,
					urlStr: b.Url,
					chkUrl: b.parsed.Scheme + "://" + b.parsed.Host + route,
				})
			}
		}
		configMu.RUnlock()

		for _, check := range checks {
			res, err := client.Get(check.chkUrl)
			isHealthy := err == nil && res.StatusCode == http.StatusOK
			if res != nil {
				res.Body.Close()
			}

			configMu.Lock()
			if check.idx < len(Configuration.Backends) && Configuration.Backends[check.idx].Url == check.urlStr {
				Configuration.Backends[check.idx].Health = isHealthy
			}
			configMu.Unlock()
		}

		configMu.RLock()
		fmt.Printf("Health Check Completed. Total Backends: %d\n", len(Configuration.Backends))
		configMu.RUnlock()
	}
}
