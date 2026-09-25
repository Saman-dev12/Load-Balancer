package config

import (
	"net/url"
	"sync/atomic"

	"github.com/Saman-dev12/lb/internal/proxy"
)

type Backend struct {
	Url        string
	Health     bool
	Parsed     *url.URL
	ActiveConn atomic.Int64 `json:"-" yaml:"-"`
	Proxy      *proxy.ReverseProxy
}

