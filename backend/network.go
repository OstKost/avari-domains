package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"time"

	"github.com/miekg/dns"
)

func publicIP(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsMulticast() || a.IsUnspecified() {
		return false
	}
	blocked := []string{"0.0.0.0/8", "100.64.0.0/10", "169.254.0.0/16", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "2001:db8::/32", "2001::/23", "2001:10::/28", "2001:20::/28", "fc00::/7", "fe80::/10", "ff00::/8"}
	for _, s := range blocked {
		p := netip.MustParsePrefix(s)
		if p.Contains(a) {
			return false
		}
	}
	return true
}
func resolvedPublic(ctx context.Context, host string) ([]net.IP, error) {
	type answer struct {
		ips []net.IP
		err error
	}
	results := make(chan answer, 2)
	for _, kind := range []uint16{dns.TypeA, dns.TypeAAAA} {
		go func(qtype uint16) {
			rr, _, err := records(ctx, host, qtype)
			ips := []net.IP{}
			for _, v := range rr {
				switch x := v.(type) {
				case *dns.A:
					ips = append(ips, x.A)
				case *dns.AAAA:
					ips = append(ips, x.AAAA)
				}
			}
			results <- answer{ips, err}
		}(kind)
	}
	ips := []net.IP{}
	var firstErr error
	for range 2 {
		a := <-results
		if firstErr == nil {
			firstErr = a.err
		}
		ips = append(ips, a.ips...)
	}
	if len(ips) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, errors.New("DNS не вернул адрес")
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return nil, errors.New("внутренний или специальный IP заблокирован")
		}
	}
	sort.SliceStable(ips, func(i, j int) bool { return ips[i].To4() != nil && ips[j].To4() == nil })
	return ips, nil
}
func dialPublic(ctx context.Context, ips []net.IP, port string) (net.Conn, error) {
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var last error
	for _, ip := range ips {
		d := net.Dialer{Timeout: 2 * time.Second}
		conn, err := d.DialContext(c, "tcp", net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
		if c.Err() != nil {
			break
		}
	}
	return nil, last
}
func pinnedTransport(ctx context.Context, host string, port string, skipTLS bool) (*http.Transport, error) {
	ips, err := resolvedPublic(ctx, host)
	if err != nil {
		return nil, err
	}
	return &http.Transport{Proxy: nil, DisableKeepAlives: true, TLSClientConfig: &tls.Config{ServerName: host, InsecureSkipVerify: skipTLS}, DialContext: func(c context.Context, network, address string) (net.Conn, error) {
		return dialPublic(c, ips, port)
	}}, nil
}
func safeURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" || u.User != nil || net.ParseIP(u.Hostname()) != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("недопустимый URL")
	}
	if p := u.Port(); p != "" && p != "80" && p != "443" {
		return nil, errors.New("недопустимый порт")
	}
	return u, nil
}
func requestPinned(ctx context.Context, rawURL string, method string, limit int) (int, http.Header, []byte, error) {
	opCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	u, err := safeURL(rawURL)
	if err != nil {
		return 0, nil, nil, err
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if port != "80" && port != "443" {
		return 0, nil, nil, errors.New("недопустимый порт")
	}
	tr, err := pinnedTransport(opCtx, u.Hostname(), port, false)
	if err != nil {
		return 0, nil, nil, err
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Timeout: 5 * time.Second, Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(opCtx, method, rawURL, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "AvariDomains/1.0 (+domain diagnostics)")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)))
	return resp.StatusCode, resp.Header, body, err
}
func requestPorkbun(ctx context.Context, domain string) ([]byte, error) {
	opCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	host := "api.porkbun.com"
	tr, err := pinnedTransport(opCtx, host, "443", false)
	if err != nil {
		return nil, err
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Timeout: 5 * time.Second, Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(opCtx, "POST", "https://"+host+"/api/json/v3/domain/checkDomain/"+url.PathEscape(domain), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", os.Getenv("PORKBUN_API_KEY"))
	req.Header.Set("X-Secret-API-Key", os.Getenv("PORKBUN_SECRET_API_KEY"))
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Porkbun HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 65536))
}
func followHTTP(ctx context.Context, raw string) Finding {
	return followHTTPWith(ctx, raw, func(c context.Context, target string) (int, http.Header, error) {
		code, h, _, err := requestPinned(c, target, "GET", 4096)
		return code, h, err
	})
}
func followHTTPWith(ctx context.Context, raw string, fetch func(context.Context, string) (int, http.Header, error)) Finding {
	f := Finding{CheckedAt: time.Now().UTC(), Status: "unknown"}
	seen := map[string]bool{}
	current := raw
	chain := []string{}
	downgrade := false
	for i := 0; i <= 5; i++ {
		if seen[current] {
			f.Summary = "Цикл редиректов"
			f.Status = "warning"
			break
		}
		seen[current] = true
		if _, err := safeURL(current); err != nil {
			f.Summary = "Небезопасный редирект"
			f.Status = "warning"
			f.Detail = err.Error()
			break
		}
		code, headers, err := fetch(ctx, current)
		if err != nil {
			f.Summary = "Не удалось подключиться"
			f.Detail = err.Error()
			break
		}
		chain = append(chain, fmt.Sprintf("%d %s", code, current))
		if code == 403 || code == 429 {
			f.Status = "warning"
			f.Summary = "Сервер ограничил доступ"
			break
		}
		if code >= 300 && code < 400 && headers.Get("Location") != "" {
			next, err := url.Parse(headers.Get("Location"))
			if err != nil {
				f.Summary = "Некорректный редирект"
				break
			}
			base, _ := url.Parse(current)
			dest := base.ResolveReference(next)
			if dest.Scheme != "http" && dest.Scheme != "https" {
				f.Summary = "Небезопасный редирект"
				break
			}
			if base.Scheme == "https" && dest.Scheme == "http" {
				downgrade = true
			}
			current = dest.String()
			if i == 5 {
				f.Summary = "Слишком много редиректов"
				f.Status = "warning"
			}
			continue
		}
		if code >= 200 && code < 400 {
			f.Status = "ok"
			f.Summary = "Сайт отвечает"
		} else {
			f.Status = "warning"
			f.Summary = "Сервер вернул ошибку"
		}
		if downgrade {
			f.Status = "warning"
			f.Detail = "Обнаружен переход HTTPS → HTTP"
		}
		if rawURLScheme(raw) == "https" && f.Status == "ok" && headers.Get("Strict-Transport-Security") != "" {
			f.Detail = "HSTS включён: " + headers.Get("Strict-Transport-Security")
		}
		break
	}
	f.Values = chain
	return f
}
func rawURLScheme(s string) string {
	u, _ := url.Parse(s)
	if u == nil {
		return ""
	}
	return u.Scheme
}
