// Package subscription downloads subscription links and probes servers.
package subscription

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/damoonartwork/phantomart/internal/link"
)

const userAgent = "PhantomArt/0.1 (v2rayN compatible)"

// Fetch downloads a subscription and returns the profiles it contains.
// If proxyURL is non-empty the request goes through it (e.g. the running core).
func Fetch(ctx context.Context, subURL, proxyURL string) ([]*link.Profile, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	if proxyURL != "" {
		pu, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}
		client.Transport = &http.Transport{Proxy: http.ProxyURL(pu)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subscription returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	profiles := link.ParseMany(link.DecodeSubscription(body))
	for _, p := range profiles {
		p.Subscription = subURL
	}
	return profiles, nil
}

// TCPPing measures how long a TCP handshake to the server takes, in ms.
// It returns -1 if the server is unreachable.
func TCPPing(p *link.Profile, timeout time.Duration) int {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(p.Address, strconv.Itoa(p.Port)), timeout)
	if err != nil {
		return -1
	}
	conn.Close()
	return int(time.Since(start).Milliseconds())
}

// RealDelay fetches a tiny URL through the local HTTP proxy and returns the
// round-trip time in ms. This verifies the whole tunnel actually works.
func RealDelay(ctx context.Context, httpProxy string) (int, error) {
	pu, err := url.Parse(httpProxy)
	if err != nil {
		return -1, err
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(pu), DisableKeepAlives: true},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.gstatic.com/generate_204", nil)
	if err != nil {
		return -1, err
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return -1, fmt.Errorf("unexpected status %s", resp.Status)
	}
	return int(time.Since(start).Milliseconds()), nil
}
