package loadbalancer

import (
	"fmt"
	"net/http"
	"time"
)

type checkData struct {
	idx    int
	urlStr string
	chkUrl string
}

func HealthCheck() {
	ticker := time.NewTicker(time.Duration(Configuration.Duration) * time.Second)
	defer ticker.Stop()

	client := &http.Client{Timeout: 2 * time.Second}

	for range ticker.C {
		ConfigMu.RLock()

		var checks []checkData
		route := Configuration.HealthCheckRoute

		for i, b := range Configuration.Backends {
			if b.Parsed != nil {
				checks = append(checks, checkData{
					idx:    i,
					urlStr: b.Url,
					chkUrl: b.Parsed.Scheme + "://" + b.Parsed.Host + route,
				})
			}
		}
		ConfigMu.RUnlock()

		for _, check := range checks {
			res, err := client.Get(check.chkUrl)
			isHealthy := err == nil && res.StatusCode == http.StatusOK
			if res != nil {
				res.Body.Close()
			}

			ConfigMu.Lock()
			if check.idx < len(Configuration.Backends) && Configuration.Backends[check.idx].Url == check.urlStr {
				Configuration.Backends[check.idx].Health = isHealthy
			}
			ConfigMu.Unlock()
		}

		ConfigMu.RLock()
		fmt.Printf("Health Check Completed. Total Backends: %d\n", len(Configuration.Backends))
		ConfigMu.RUnlock()
	}
}
