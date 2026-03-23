package config

import (
	"net/http/httputil"
	"net/url"
	"sync/atomic"
)

type Backend struct {
	Url        string
	Health     bool
	Parsed     *url.URL
	ActiveConn atomic.Int64 `json:"-" yaml:"-"`
	Proxy      *httputil.ReverseProxy
}
