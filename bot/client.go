package bot

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const UnaryTimeout = 30 * time.Second

type unaryTimeoutTransport struct {
	base  *http.Transport
	unary time.Duration
}

func (t *unaryTimeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.base == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	if t.unary > 0 && req.URL != nil && !strings.Contains(req.URL.Path, "getUpdates") {
		ctx, cancel := context.WithTimeout(req.Context(), t.unary)
		defer cancel()
		req = req.WithContext(ctx)
	}
	return t.base.RoundTrip(req)
}

func NewHTTPClient(proxyURL, user, password string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSHandshakeTimeout = 20 * time.Second
	// 0: иначе HTTP/2 long-poll getUpdates падает с "timeout awaiting response headers"
	transport.ResponseHeaderTimeout = 0
	transport.IdleConnTimeout = 90 * time.Second
	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("некорректный TG_PROXY: %w", err)
		}
		if user != "" {
			u.User = url.UserPassword(user, password)
		}
		transport.Proxy = http.ProxyURL(u)
	}
	return &http.Client{
		Timeout: 0,
		Transport: &unaryTimeoutTransport{
			base:  transport,
			unary: UnaryTimeout,
		},
	}, nil
}

func innerTransport(rt http.RoundTripper) *http.Transport {
	switch t := rt.(type) {
	case *http.Transport:
		return t
	case *unaryTimeoutTransport:
		return t.base
	default:
		return nil
	}
}

func ProxyHost(proxyURL string) string {
	u, err := url.Parse(proxyURL)
	if err != nil || u.Host == "" {
		return proxyURL
	}
	if u.Scheme == "" {
		return u.Host
	}
	return u.Scheme + "://" + u.Host
}

func ProxySecrets(proxyURL, user, password string) []string {
	var out []string
	if user != "" {
		out = append(out, user)
	}
	if password != "" {
		out = append(out, password)
	}
	u, err := url.Parse(proxyURL)
	if err != nil || u.User == nil {
		return out
	}
	if name := u.User.Username(); name != "" {
		out = append(out, name)
	}
	if pass, ok := u.User.Password(); ok && pass != "" {
		out = append(out, pass)
	}
	return out
}

func Redact(err error, secrets ...string) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "***")
		}
	}
	return s
}
