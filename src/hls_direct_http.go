package src

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func validateDirectHLSURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid upstream URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("unsupported upstream protocol")
	}
	if u.Hostname() == "" || u.User != nil {
		return errors.New("invalid upstream host")
	}

	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return errors.New("invalid upstream port")
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return errors.New("invalid upstream port")
	}

	return nil
}

func directHLSResolvePublic(
	ctx context.Context,
	hostname string,
) (string, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil || len(ips) == 0 {
		return "", errors.New("unable to resolve upstream host")
	}

	for _, entry := range ips {
		if !directHLSPublicIP(entry.IP) {
			return "", errors.New("upstream host resolves to a restricted address")
		}
	}

	return ips[0].IP.String(), nil
}

func newDirectHLSClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   8 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy:                 nil,
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 15 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}

			ip, err := directHLSResolvePublic(ctx, host)
			if err != nil {
				return nil, err
			}

			return dialer.DialContext(
				ctx,
				network,
				net.JoinHostPort(ip, port),
			)
		},
	}

	return &http.Client{
		Transport: transport,
		Timeout:   35 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many upstream redirects")
			}
			if err := validateDirectHLSURL(req.URL.String()); err != nil {
				return fmt.Errorf("redirect rejected: %w", err)
			}
			return nil
		},
	}
}
