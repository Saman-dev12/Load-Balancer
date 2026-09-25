package loadbalancer

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type checkData struct {
	idx    int
	urlStr string
	chkUrl string
}

func HealthCheck() {
	ConfigMu.RLock()
	duration := Configuration.Duration
	ConfigMu.RUnlock()
	if duration <= 0 {
		duration = 10
	}

	ticker := time.NewTicker(time.Duration(duration) * time.Second)
	defer ticker.Stop()

	client := &http.Client{Timeout: 2 * time.Second}

	for range ticker.C {
		ConfigMu.RLock()
		var checks []checkData
		route := Configuration.HealthCheckRoute
		if !strings.HasPrefix(route, "/") {
			route = "/" + route
		}

		for i, b := range Configuration.Backends {
			if b.Parsed != nil {
				chkURL := b.Parsed.ResolveReference(&url.URL{Path: route}).String()
				checks = append(checks, checkData{
					idx:    i,
					urlStr: b.Url,
					chkUrl: chkURL,
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
