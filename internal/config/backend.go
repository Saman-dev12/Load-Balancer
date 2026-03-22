package config

import (
	"net/http/httputil"
	"net/url"
)

type Backend struct {
	Url        string
	Health     bool
	Parsed     *url.URL
	ActiveConn int64
	Proxy      *httputil.ReverseProxy
}
