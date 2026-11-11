package providers

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Remote output URLs never receive provider credentials and cannot reach local services.
// Explicit loopback mock API bases are allowed only by constructor-injected test clients.
func publicMedia(ctx context.Context, raw string, limit int64, apiBase ...string) ([]byte, error) {
	allowLocal := false
	if len(apiBase) > 0 {
		base, err := url.Parse(apiBase[0])
		allowLocal = err == nil && base.Scheme == "http" && net.ParseIP(base.Hostname()).IsLoopback()
	}
	validate := func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil || u.User != nil || u.Hostname() == "" || u.Fragment != "" || (u.Scheme != "https" && !(allowLocal && u.Scheme == "http")) {
			return fmt.Errorf("provider returned an invalid media URL")
		}
		if !allowLocal {
			if ip := net.ParseIP(u.Hostname()); ip != nil && !publicIP(ip) {
				return fmt.Errorf("provider returned a private media URL")
			}
		}
		return nil
	}
	if err := validate(raw); err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if !allowLocal {
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil || len(ips) == 0 {
				return nil, fmt.Errorf("cannot resolve media host")
			}
			for _, ip := range ips {
				if !publicIP(ip.IP) {
					return nil, fmt.Errorf("media host resolves to a private address")
				}
			}
			dialer := &net.Dialer{Timeout: 30 * time.Second}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		}
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 2 * time.Minute, Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many media redirects")
		}
		return validate(req.URL.String())
	}}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid media request")
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("media download failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("media download HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("media download too large")
	}
	return data, nil
}
func publicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return false
	}
	return !strings.HasPrefix(ip.String(), "2001:db8:")
}
