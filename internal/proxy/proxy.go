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

// Hop-by-hop headers defined in RFC 7230 / RFC 2616.
// These headers belong to a single transport connection and MUST NOT be forwarded by proxies.
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

// DefaultTransport is the default RoundTripper used by ReverseProxy
// configured with connection pooling, keep-alive, and dial timeouts.
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

// ReverseProxy is a custom HTTP request forwarder.
// It takes incoming HTTP requests, rewrites them for the destination backend,
// sends them to the backend, and streams the response back to the client.
type ReverseProxy struct {
	Target       *url.URL
	Transport    http.RoundTripper
	ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)
}

// NewCustomReverseProxy creates a new ReverseProxy forwarding to target.
func NewCustomReverseProxy(target *url.URL) *ReverseProxy {
	return &ReverseProxy{
		Target:    target,
		Transport: DefaultTransport,
	}
}

// ServeHTTP handles an incoming client request by forwarding it to the target backend.
func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	transport := p.Transport
	if transport == nil {
		transport = DefaultTransport
	}

	// Step 1: Construct destination URL
	outURL := *p.Target
	outURL.Path = singleJoiningSlash(p.Target.Path, r.URL.Path)
	outURL.RawQuery = r.URL.RawQuery
	outURL.Fragment = r.URL.Fragment

	// Step 2: Create outbound request with client context (handles cancellation/timeouts)
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, outURL.String(), r.Body)
	if err != nil {
		p.handleError(w, r, fmt.Errorf("failed to create outbound request: %w", err), http.StatusInternalServerError)
		return
	}

	// Preserve content length for requests with bodies (e.g. POST, PUT)
	outReq.ContentLength = r.ContentLength

	// Step 3: Copy and filter headers
	copyHeaders(outReq.Header, r.Header)
	removeHopByHopHeaders(outReq.Header)

	// Set Host header to match backend host
	outReq.Host = p.Target.Host

	// Step 4: Inject/update standard proxy forwarding headers (X-Forwarded-*)
	setForwardingHeaders(outReq, r)

	// Step 5: Send the request to the upstream backend
	resp, err := transport.RoundTrip(outReq)
	if err != nil {
		p.handleError(w, r, fmt.Errorf("backend connection error: %w", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Step 6: Filter upstream response headers and copy to client response
	removeHopByHopHeaders(resp.Header)
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Step 7: Write backend response status code
	w.WriteHeader(resp.StatusCode)

	// Step 8: Stream response body back to client
	_, copyErr := io.Copy(w, resp.Body)
	if copyErr != nil {
		// Log or ignore if connection was broken mid-stream
		return
	}

	// Flush if supported
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// handleError sends an error response when request forwarding fails.
func (p *ReverseProxy) handleError(w http.ResponseWriter, r *http.Request, err error, statusCode int) {
	if p.ErrorHandler != nil {
		p.ErrorHandler(w, r, err)
		return
	}

	// If client cancelled context, do not attempt to write
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

// copyHeaders copies all HTTP headers from src to dst.
func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// removeHopByHopHeaders strips connection-specific headers according to RFC 7230.
func removeHopByHopHeaders(h http.Header) {
	// First, check if "Connection" specifies custom hop-by-hop headers (comma-separated)
	if c := h.Get("Connection"); c != "" {
		for _, token := range strings.Split(c, ",") {
			headerName := strings.TrimSpace(token)
			if headerName != "" {
				h.Del(headerName)
			}
		}
	}

	// Remove well-known hop-by-hop headers
	for hop := range hopByHopHeaders {
		h.Del(hop)
	}
}

// setForwardingHeaders adds or updates X-Forwarded-For, X-Forwarded-Host, and X-Forwarded-Proto.
func setForwardingHeaders(outReq *http.Request, inReq *http.Request) {
	// 1. X-Forwarded-For: append client IP
	clientIP, _, err := net.SplitHostPort(inReq.RemoteAddr)
	if err != nil {
		clientIP = inReq.RemoteAddr
	}

	if prior := outReq.Header.Get("X-Forwarded-For"); prior != "" {
		outReq.Header.Set("X-Forwarded-For", prior+", "+clientIP)
	} else if clientIP != "" {
		outReq.Header.Set("X-Forwarded-For", clientIP)
	}

	// 2. X-Forwarded-Host: original host requested by the client
	if outReq.Header.Get("X-Forwarded-Host") == "" {
		if inReq.Host != "" {
			outReq.Header.Set("X-Forwarded-Host", inReq.Host)
		}
	}

	// 3. X-Forwarded-Proto: http or https
	if outReq.Header.Get("X-Forwarded-Proto") == "" {
		proto := "http"
		if inReq.TLS != nil {
			proto = "https"
		}
		outReq.Header.Set("X-Forwarded-Proto", proto)
	}
}

// singleJoiningSlash safely joins two URL path segments without double slashes.
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
