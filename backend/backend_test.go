package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestNormalizeInput(t *testing.T) {
	cases := []struct {
		input, host, domain string
		valid               bool
	}{{"https://www.пример.рф/path?q=1", "www.xn--e1afmkfd.xn--p1ai", "xn--e1afmkfd.xn--p1ai", true}, {"foo.bar.co.uk", "foo.bar.co.uk", "bar.co.uk", true}, {"https://user:pass@example.com", "", "", false}, {"http://example.com:8080", "", "", false}, {"127.0.0.1", "", "", false}, {"https://[::1]/", "", "", false}}
	for _, tc := range cases {
		h, d, err := normalizeInput(tc.input)
		if (err == nil) != tc.valid || h != tc.host || d != tc.domain {
			t.Errorf("%q: %q %q %v", tc.input, h, d, err)
		}
	}
}
func TestPublicIP(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "172.16.1.1", "192.168.1.1", "169.254.1.1", "100.64.0.1", "::1", "fc00::1", "fe80::1", "2001:db8::1", "::ffff:127.0.0.1"} {
		if publicIP(net.ParseIP(s)) {
			t.Errorf("accepted %s", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888"} {
		if !publicIP(net.ParseIP(s)) {
			t.Errorf("rejected %s", s)
		}
	}
}
func TestRateLimitBurstAndRefill(t *testing.T) {
	s := &server{limits: map[string]bucket{}}
	for i := 0; i < 10; i++ {
		if !s.allow("1.2.3.4") {
			t.Fatalf("token %d rejected", i)
		}
	}
	if s.allow("1.2.3.4") {
		t.Fatal("11th token accepted")
	}
	s.mu.Lock()
	b := s.limits["1.2.3.4"]
	b.at = b.at.Add(-3 * time.Second)
	s.limits["1.2.3.4"] = b
	s.mu.Unlock()
	if !s.allow("1.2.3.4") {
		t.Fatal("token did not refill")
	}
}
func TestStoreIsolationAndCache(t *testing.T) {
	store, err := openStore(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.db.Close()
	r := Result{ID: "first", Hostname: "example.com", Domain: "example.com", Registration: Registration{Status: "registered"}, Site: map[string]Finding{}, Mail: map[string]Finding{}, CheckedAt: time.Now()}
	if err = store.save("alice", r); err != nil {
		t.Fatal(err)
	}
	if _, err = store.item("bob", "first"); err == nil {
		t.Fatal("other session read history")
	}
	items, err := store.history("alice", 0)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	if err = store.putCache(r, time.Minute); err != nil {
		t.Fatal(err)
	}
	got, ok := store.cached("example.com")
	if !ok || !got.Cached {
		t.Fatal("cache miss")
	}
}
func TestCachedCheckUsesRateLimitAndSession(t *testing.T) {
	store, err := openStore(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.db.Close()
	base := Result{Hostname: "example.com", Domain: "example.com", Registration: Registration{Status: "registered"}, Site: map[string]Finding{}, Mail: map[string]Finding{}, CheckedAt: time.Now()}
	if err = store.putCache(base, time.Minute); err != nil {
		t.Fatal(err)
	}
	s := &server{store: store, checker: newChecker(), work: make(chan struct{}, 8), limits: map[string]bucket{}, trusted: map[string]bool{}}
	var cookie *http.Cookie
	for i := 0; i < 11; i++ {
		req := httptest.NewRequest("POST", "/api/check", strings.NewReader(`{"input":"example.com"}`))
		req.RemoteAddr = "8.8.8.8:1234"
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		s.check(w, req)
		if i == 0 {
			cookie = w.Result().Cookies()[0]
			var got Result
			if json.Unmarshal(w.Body.Bytes(), &got) != nil || !got.Cached {
				t.Fatal("cache not returned")
			}
		}
		want := 200
		if i == 10 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("request %d got %d: %s", i, w.Code, w.Body.String())
		}
		if i == 10 && w.Header().Get("Retry-After") == "" {
			t.Fatal("missing Retry-After")
		}
	}
}

func TestBootstrapSnapshot(t *testing.T) {
	c := newChecker()
	if c.bootstrap["com"] == "" || c.bootstrap["org"] == "" {
		t.Fatal("IANA bootstrap snapshot missing common TLDs")
	}
	if c.bootstrap["ru"] != "" || c.bootstrap["xn--p1ai"] != "" {
		t.Fatal("RU fallback should remain WHOIS")
	}
}

func TestCatalogPriceAndFreshness(t *testing.T) {
	first, renewal, source, date, ok := catalogPrice("org")
	if !ok || first != 7.98 || renewal != 11.84 || source == "" || date == "" {
		t.Fatalf("invalid catalog: %v %v %v %v %v", first, renewal, source, date, ok)
	}
	if _, _, _, _, ok := catalogPrice("unsupported"); ok {
		t.Fatal("unsupported zone has a price")
	}
	if priceStale("2026-09-28", time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)) == false {
		t.Fatal("8-day tariff should be stale")
	}
	if priceStale("2026-09-28", time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("fresh tariff marked stale")
	}
}

func TestRegistrationResponses(t *testing.T) {
	now := time.Now()
	if classifyRDAP(200, now).Status != "registered" {
		t.Fatal("RDAP 200")
	}
	if classifyRDAP(404, now).Status != "unregistered" {
		t.Fatal("RDAP 404")
	}
	if classifyRDAP(429, now).Status != "unknown" {
		t.Fatal("RDAP 429 must remain unknown")
	}
	if classifyWHOIS("example.ru", "domain: example.ru\nstate: REGISTERED", now).Status != "registered" {
		t.Fatal("WHOIS registered")
	}
	if classifyWHOIS("example.ru", "No entries found", now).Status != "unregistered" {
		t.Fatal("WHOIS empty")
	}
	if classifyWHOIS("example.ru", "Try again later", now).Status != "unknown" {
		t.Fatal("WHOIS unknown")
	}
}

func TestRedirectsAndAccessLimits(t *testing.T) {
	calls := 0
	internal := followHTTPWith(context.Background(), "https://example.com/", func(_ context.Context, _ string) (int, http.Header, error) {
		calls++
		return 301, http.Header{"Location": []string{"http://127.0.0.1/"}}, nil
	})
	if internal.Status != "warning" || calls != 1 {
		t.Fatalf("internal redirect: %+v, calls=%d", internal, calls)
	}
	cycle := followHTTPWith(context.Background(), "https://example.com/", func(_ context.Context, _ string) (int, http.Header, error) {
		return 301, http.Header{"Location": []string{"/"}}, nil
	})
	if cycle.Status != "warning" || !strings.Contains(cycle.Summary, "Цикл") {
		t.Fatalf("cycle: %+v", cycle)
	}
	downgrade := followHTTPWith(context.Background(), "https://example.com/", func(_ context.Context, u string) (int, http.Header, error) {
		if strings.HasPrefix(u, "https:") {
			return 302, http.Header{"Location": []string{"http://example.com/"}}, nil
		}
		return 200, http.Header{}, nil
	})
	if downgrade.Status != "warning" || !strings.Contains(downgrade.Detail, "HTTPS") {
		t.Fatalf("downgrade: %+v", downgrade)
	}
	limited := followHTTPWith(context.Background(), "https://example.com/", func(_ context.Context, _ string) (int, http.Header, error) { return 403, http.Header{}, nil })
	if limited.Status != "warning" || !strings.Contains(limited.Summary, "ограничил") {
		t.Fatalf("403: %+v", limited)
	}
}

func TestOfferCatalogAndComparison(t *testing.T) {
	namecheap := catalogOffer("Namecheap", "example.com", "com")
	timeweb := catalogOffer("Timeweb", "example.com", "com")
	godaddy := catalogOffer("GoDaddy", "example.com", "com")
	if namecheap.FirstYear == nil || *namecheap.FirstYear != 11.28 || namecheap.Renewal == nil || *namecheap.Renewal != 18.48 || !namecheap.Promo {
		t.Fatalf("Namecheap catalog: %+v", namecheap)
	}
	if timeweb.FirstYear == nil || *timeweb.FirstYear != 1560 || timeweb.Currency != "RUB" {
		t.Fatalf("Timeweb catalog: %+v", timeweb)
	}
	if godaddy.FirstYear != nil || godaddy.Source == "" || godaddy.SourceDate == "" {
		t.Fatalf("unverified offer: %+v", godaddy)
	}
	porkbun := catalogOffer("Porkbun", "example.com", "com")
	offers := markBestOffers([]Offer{porkbun, namecheap, timeweb, godaddy})
	if offers[0].Badge != "Минимальный тариф от" || offers[1].Badge != "Минимальный тариф от" || offers[2].Badge != "Минимальный тариф от" || offers[3].Badge != "" {
		t.Fatalf("bad comparison: %+v", offers)
	}
	offers[0].Stale = true
	offers = markBestOffers(offers)
	if offers[0].Badge != "" {
		t.Fatal("stale price won comparison")
	}
}

func TestMailChecksWithControlledDNS(t *testing.T) {
	rr := func(s string) dns.RR {
		r, err := dns.NewRR(s)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	answers := map[string][]dns.RR{
		"mail.example.com:15":                   {rr("mail.example.com. 300 IN MX 0 .")},
		"mail.example.com:16":                   {rr("mail.example.com. 300 IN TXT \"v=spf1 -all\""), rr("mail.example.com. 300 IN TXT \"v=spf1 +mx\"")},
		"google._domainkey.mail.example.com:16": {rr("google._domainkey.mail.example.com. 300 IN TXT \"v=DKIM1; p=abcd\"")},
		"_dmarc.example.com:16":                 {rr("_dmarc.example.com. 300 IN TXT \"v=DMARC1; p=reject\"")},
	}
	lookup := func(_ context.Context, name string, kind uint16) ([]dns.RR, int, error) {
		return answers[name+":"+strconv.Itoa(int(kind))], dns.RcodeSuccess, nil
	}
	got := mailChecksWith(context.Background(), "mail.example.com", "example.com", lookup)
	if got["mx"].Status != "neutral" || got["spf"].Status != "warning" || got["dkim"].Status != "ok" || !strings.Contains(got["dmarc"].Summary, "Наследуется") {
		t.Fatalf("mail checks: %+v", got)
	}
	empty := mailChecksWith(context.Background(), "empty.example.com", "example.com", func(_ context.Context, _ string, _ uint16) ([]dns.RR, int, error) { return nil, dns.RcodeSuccess, nil })
	if empty["dkim"].Status != "neutral" || !strings.Contains(empty["dkim"].Summary, "среди проверенных") {
		t.Fatalf("DKIM absence: %+v", empty["dkim"])
	}
	partial := mailChecksWith(context.Background(), "mail.example.com", "example.com", func(ctx context.Context, name string, kind uint16) ([]dns.RR, int, error) {
		if kind == dns.TypeMX {
			return nil, 0, errors.New("timeout")
		}
		return lookup(ctx, name, kind)
	})
	if partial["mx"].Status != "unknown" || partial["dkim"].Status != "ok" {
		t.Fatalf("partial timeout: %+v", partial)
	}
}

func TestNSComparison(t *testing.T) {
	if compareNS("example.com", []string{"a.example.net"}, []string{"a.example.net"}).Status != "ok" {
		t.Fatal("matching NS")
	}
	if compareNS("example.com", []string{"a.example.net"}, []string{"b.example.net"}).Status != "warning" {
		t.Fatal("NS mismatch")
	}
	if compareNS("example.com", []string{"a.example.net"}, nil).Status != "unknown" {
		t.Fatal("unavailable authoritative NS")
	}
}
