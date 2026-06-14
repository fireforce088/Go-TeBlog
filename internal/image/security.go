package image

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

func NewSSRFHTTPClient(resolver Resolver, dial DialContextFunc, timeout time.Duration) *http.Client {
	transport := SSRFTransport(resolver, dial)
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after 5 redirects")
			}
			if err := validateRemoteURL(req.URL); err != nil {
				return err
			}
			_, err := resolveAndValidate(req.Context(), resolver, req.URL.Hostname())
			return err
		},
	}
}

func SSRFTransport(resolver Resolver, dial DialContextFunc) *http.Transport {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if dial == nil {
		dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
		dial = dialer.DialContext
	}
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := resolveAndValidate(ctx, resolver, host)
			if err != nil {
				return nil, err
			}
			return dial(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
		DialTLSContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := resolveAndValidate(ctx, resolver, host)
			if err != nil {
				return nil, err
			}
			conn, err := dial(ctx, network, net.JoinHostPort(ips[0].String(), port))
			if err != nil {
				return nil, err
			}
			tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				if closeErr := conn.Close(); closeErr != nil {
					_ = closeErr
				}
				return nil, err
			}
			return tlsConn, nil
		},
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:   true,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}

func validateRemoteURL(u *url.URL) error {
	if u == nil {
		return fmt.Errorf("missing URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("URL userinfo is not allowed")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("missing host")
	}
	if !strings.Contains(host, ".") && net.ParseIP(host) == nil {
		return fmt.Errorf("single-label host is not allowed")
	}
	return nil
}

func resolveAndValidate(ctx context.Context, resolver Resolver, host string) ([]net.IP, error) {
	if err := validateHost(host); err != nil {
		return nil, err
	}
	var parsed []net.IP
	if ip := net.ParseIP(host); ip != nil {
		parsed = append(parsed, ip)
	} else {
		hosts, err := resolver.LookupHost(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, h := range hosts {
			ip := net.ParseIP(h)
			if ip == nil {
				return nil, fmt.Errorf("resolver returned non-IP %q", h)
			}
			parsed = append(parsed, ip)
		}
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("host resolved to no addresses")
	}
	for _, ip := range parsed {
		if isBlockedIP(ip) {
			return nil, fmt.Errorf("blocked private or reserved address %s", ip.String())
		}
	}
	return parsed, nil
}

func validateHost(host string) error {
	if host == "" {
		return fmt.Errorf("missing host")
	}
	if !strings.Contains(host, ".") && net.ParseIP(host) == nil {
		return fmt.Errorf("single-label host is not allowed")
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	blockedCIDRs := []string{
		"0.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"224.0.0.0/4",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
		"ff00::/8",
	}
	for _, cidr := range blockedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
