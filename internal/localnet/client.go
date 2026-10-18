// Package localnet confines daemon HTTP clients to literal loopback addresses.
package localnet

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"
)

var ErrRemote = errors.New("outbound network access is disabled; only literal loopback daemon addresses are supported")

func Check(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return ErrRemote
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return ErrRemote
	}
	return nil
}

// Client ignores proxy environment variables and refuses redirects. A supplied
// client may contribute timeouts and TLS roots, but cannot replace the dialer.
func Client(base string, original *http.Client) (*http.Client, error) {
	if err := Check(base); err != nil {
		return nil, err
	}
	c := http.Client{Timeout: 30 * time.Second}
	if original != nil {
		c = *original
	}
	tr := &http.Transport{TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second}
	if original != nil {
		if old, ok := original.Transport.(*http.Transport); ok && old.TLSClientConfig != nil {
			tr.TLSClientConfig = old.TLSClientConfig.Clone()
		}
	}
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil, ErrRemote
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return nil, ErrRemote
		}
		return (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, address)
	}
	c.Transport = transport{tr}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c, nil
}

type transport struct{ next *http.Transport }

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := Check(r.URL.String()); err != nil {
		return nil, err
	}
	return t.next.RoundTrip(r)
}
func (t transport) CloseIdleConnections() { t.next.CloseIdleConnections() }
