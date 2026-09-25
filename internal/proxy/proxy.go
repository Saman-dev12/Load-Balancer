package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Proxy-Connection":    true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Trailers":            true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

var DefaultTransport http.RoundTripper = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ResponseHeaderTimeout: 15 * time.Second,
}

type ReverseProxy struct {
	Target       *url.URL
	Transport    http.RoundTripper
	ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)
}

func NewCustomReverseProxy(target *url.URL) *ReverseProxy {
	return &ReverseProxy{
		Target:    target,
		Transport: DefaultTransport,
	}
}

func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	transport := p.Transport
	if transport == nil {
		transport = DefaultTransport
	}

	outURL := *p.Target
	outURL.Path = singleJoiningSlash(p.Target.Path, r.URL.Path)
	outURL.RawQuery = r.URL.RawQuery
	outURL.Fragment = r.URL.Fragment

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, outURL.String(), r.Body)
	if err != nil {
		p.handleError(w, r, fmt.Errorf("failed to create outbound request: %w", err), http.StatusInternalServerError)
		return
	}

	outReq.ContentLength = r.ContentLength

	copyHeaders(outReq.Header, r.Header)
	removeHopByHopHeaders(outReq.Header)

	outReq.Host = p.Target.Host

	setForwardingHeaders(outReq, r)

	resp, err := transport.RoundTrip(outReq)
	if err != nil {
		p.handleError(w, r, fmt.Errorf("backend connection error: %w", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	removeHopByHopHeaders(resp.Header)
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		return
	}

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (p *ReverseProxy) handleError(w http.ResponseWriter, r *http.Request, err error, statusCode int) {
	if p.ErrorHandler != nil {
		p.ErrorHandler(w, r, err)
		return
	}

	if r.Context().Err() != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   http.StatusText(statusCode),
		"message": err.Error(),
	})
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func removeHopByHopHeaders(h http.Header) {
	if c := h.Get("Connection"); c != "" {
		for _, token := range strings.Split(c, ",") {
			headerName := strings.TrimSpace(token)
			if headerName != "" {
				h.Del(headerName)
			}
		}
	}

	for hop := range hopByHopHeaders {
		h.Del(hop)
	}
}

func setForwardingHeaders(outReq *http.Request, inReq *http.Request) {
	clientIP, _, err := net.SplitHostPort(inReq.RemoteAddr)
	if err != nil {
		clientIP = inReq.RemoteAddr
	}

	if prior := outReq.Header.Get("X-Forwarded-For"); prior != "" {
		outReq.Header.Set("X-Forwarded-For", prior+", "+clientIP)
	} else if clientIP != "" {
		outReq.Header.Set("X-Forwarded-For", clientIP)
	}

	if outReq.Header.Get("X-Forwarded-Host") == "" && inReq.Host != "" {
		outReq.Header.Set("X-Forwarded-Host", inReq.Host)
	}

	if outReq.Header.Get("X-Forwarded-Proto") == "" {
		proto := "http"
		if inReq.TLS != nil {
			proto = "https"
		}
		outReq.Header.Set("X-Forwarded-Proto", proto)
	}
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		if a == "" || b == "" {
			return a + b
		}
		return a + "/" + b
	}
	return a + b
}
