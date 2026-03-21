package httpserver

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/config"
)

const clerkFrontendAPIProxyPath = "/__clerk"

func newClerkFrontendAPIProxyHandler(cfg config.ClerkConfig) (http.Handler, error) {
	if strings.TrimSpace(cfg.ProxyURL) == "" {
		return nil, nil
	}

	targetURL, err := url.Parse(cfg.FrontendAPIURL)
	if err != nil {
		return nil, err
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(proxyReq *httputil.ProxyRequest) {
			proxyReq.SetURL(targetURL)
			proxyReq.Out.Host = targetURL.Host
			proxyReq.Out.URL.Path = joinClerkProxyTargetPath(targetURL.Path, stripClerkProxyPrefix(proxyReq.In.URL.Path))
			proxyReq.Out.URL.RawPath = proxyReq.Out.URL.EscapedPath()
			proxyReq.Out.Header.Set("Clerk-Proxy-Url", absoluteClerkProxyURL(proxyReq.In, cfg.ProxyURL))
			proxyReq.Out.Header.Set("Clerk-Secret-Key", cfg.SecretKey)
			proxyReq.Out.Header.Set("X-Forwarded-For", requestClientIP(proxyReq.In))
		},
	}

	return proxy, nil
}

func stripClerkProxyPrefix(path string) string {
	trimmed := strings.TrimPrefix(path, clerkFrontendAPIProxyPath)
	if trimmed == "" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		return "/" + trimmed
	}
	return trimmed
}

func joinClerkProxyTargetPath(basePath string, requestPath string) string {
	basePath = strings.TrimSuffix(basePath, "/")
	if requestPath == "" || requestPath == "/" {
		if basePath == "" {
			return "/"
		}
		return basePath
	}
	if basePath == "" {
		return requestPath
	}
	return basePath + requestPath
}

func absoluteClerkProxyURL(r *http.Request, proxyURL string) string {
	trimmed := strings.TrimSpace(proxyURL)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}

	scheme := "http"
	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = forwardedProto
	} else if r.TLS != nil {
		scheme = "https"
	}

	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}

	return fmt.Sprintf("%s://%s%s", scheme, host, trimmed)
}

func requestClientIP(r *http.Request) string {
	if forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		first := strings.Split(forwardedFor, ",")[0]
		if candidate := strings.TrimSpace(first); candidate != "" {
			return candidate
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}

	return strings.TrimSpace(r.RemoteAddr)
}
