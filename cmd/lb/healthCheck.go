package main

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func HealthCheck() {
	ticker := time.NewTicker(time.Duration(Configuration.Duration) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		configMu.RLock()
		urls := make([]*url.URL, len(Configuration.Backends))
		route := Configuration.HealthCheckRoute

		for i, b := range Configuration.Backends {
			parsedUrl, err := url.Parse(b.Url)
			if err != nil {
				continue
			}
			urls[i] = parsedUrl
		}
		configMu.RUnlock()

		for i, url := range urls {
			if url == nil {
				configMu.Lock()
				Configuration.Backends[i].Health = false
				configMu.Unlock()
				continue
			}

			healthUrl := url.Scheme + "://" + url.Host + route
			res, err := http.Get(healthUrl)

			configMu.Lock()
			if err != nil {
				Configuration.Backends[i].Health = false
				configMu.Unlock()
				continue
			}

			res.Body.Close()

			if res.StatusCode == http.StatusOK {
				Configuration.Backends[i].Health = true
			} else {
				Configuration.Backends[i].Health = false
			}
			configMu.Unlock()
		}
		configMu.RLock()
		fmt.Print(Configuration.Backends)
		configMu.RUnlock()
	}
}
