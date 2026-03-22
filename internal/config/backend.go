package config

import (
	"net/http/httputil"
	"net/url"
)

type Backend struct {
	Url    string
	Health bool
	Parsed *url.URL
	Proxy  *httputil.ReverseProxy
}
