package main

import (
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

var selectors = []string{"default", "selector1", "selector2", "google", "k1", "k2", "mail", "smtp", "dkim", "s1", "s2"}

type Checker struct {
	bootstrapMu sync.Mutex
	bootstrap   map[string]string
	loaded      time.Time
}

//go:embed rdap-bootstrap.json
var bootstrapJSON []byte

func bootstrapMap(body []byte) map[string]string {
	var data struct {
		Services [][][]string `json:"services"`
	}
	if json.Unmarshal(body, &data) != nil {
		return nil
	}
	m := map[string]string{}
	for _, svc := range data.Services {
		if len(svc) < 2 || len(svc[1]) == 0 {
			continue
		}
		for _, t := range svc[0] {
			m[t] = svc[1][0]
		}
	}
	return m
}
func newChecker() *Checker {
	return &Checker{bootstrap: bootstrapMap(bootstrapJSON), loaded: time.Now()}
}
func finding(status, summary string, values ...string) Finding {
	return Finding{Status: status, Summary: summary, Values: values, CheckedAt: time.Now().UTC()}
}
func (c *Checker) run(ctx context.Context, input, host, domain string) Result {
	r := Result{Input: input, Hostname: host, Domain: domain, Site: map[string]Finding{}, Mail: map[string]Finding{}, Offers: []Offer{}, CheckedAt: time.Now().UTC(), Region: env("CHECK_REGION", "не указан")}
	var mu sync.Mutex
	var wg sync.WaitGroup
	tasks := []func(){func() { x := c.registration(ctx, domain); mu.Lock(); r.Registration = x; mu.Unlock() }, func() {
		x := siteDNS(ctx, host, domain)
		mu.Lock()
		for k, v := range x {
			r.Site[k] = v
		}
		mu.Unlock()
	}, func() { x := siteTLS(ctx, host); mu.Lock(); r.Site["tls"] = x; mu.Unlock() }, func() { x := followHTTP(ctx, "http://"+host+"/"); mu.Lock(); r.Site["http"] = x; mu.Unlock() }, func() { x := followHTTP(ctx, "https://"+host+"/"); mu.Lock(); r.Site["https"] = x; mu.Unlock() }, func() { x := mailChecks(ctx, host, domain); mu.Lock(); r.Mail = x; mu.Unlock() }, func() { x := offers(ctx, domain); mu.Lock(); r.Offers = x; mu.Unlock() }}
	for _, task := range tasks {
		wg.Add(1)
		go func(f func()) { defer wg.Done(); f() }(task)
	}
	wg.Wait()
	if r.Registration.Status == "unregistered" || r.Registration.Status == "unknown" {
		for _, o := range r.Offers {
			if o.Kind == "exact" {
				r.Registration.Status = "available"
				r.Registration.Detail = "Доступность подтверждена регистратором Porkbun"
				break
			}
		}
	}
	return r
}
func query(ctx context.Context, name string, qtype uint16, server string) ([]dns.RR, int, error) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	m.SetEdns0(1232, true)
	cl := &dns.Client{Timeout: 4 * time.Second}
	res, _, err := cl.ExchangeContext(ctx, m, net.JoinHostPort(server, "53"))
	if err == nil && res.Truncated {
		cl.Net = "tcp"
		res, _, err = cl.ExchangeContext(ctx, m, net.JoinHostPort(server, "53"))
	}
	if err != nil {
		return nil, 0, err
	}
	if res.Rcode != dns.RcodeSuccess {
		return nil, res.Rcode, nil
	}
	if qtype == dns.TypeNS && len(res.Answer) == 0 && len(res.Ns) > 0 {
		return res.Ns, res.Rcode, nil
	}
	return res.Answer, res.Rcode, nil
}
func records(ctx context.Context, name string, qtype uint16) ([]dns.RR, int, error) {
	return query(ctx, name, qtype, env("DNS_RESOLVER", "1.1.1.1"))
}
func rrValues(rr []dns.RR) []string {
	out := []string{}
	for _, x := range rr {
		switch v := x.(type) {
		case *dns.A:
			out = append(out, v.A.String())
		case *dns.AAAA:
			out = append(out, v.AAAA.String())
		case *dns.CNAME:
			out = append(out, strings.TrimSuffix(v.Target, "."))
		case *dns.NS:
			out = append(out, strings.TrimSuffix(v.Ns, "."))
		case *dns.MX:
			out = append(out, fmt.Sprintf("%d %s", v.Preference, strings.TrimSuffix(v.Mx, ".")))
		case *dns.TXT:
			out = append(out, strings.Join(v.Txt, ""))
		}
	}
	sort.Strings(out)
	return out
}
func dnsFinding(ctx context.Context, name string, qtype uint16, optional bool) Finding {
	rr, rcode, err := records(ctx, name, qtype)
	if err != nil {
		return finding("unknown", "DNS-запрос не выполнен", err.Error())
	}
	if rcode == dns.RcodeNameError {
		return finding("missing", "Имя не найдено в DNS")
	}
	if rcode != dns.RcodeSuccess {
		return finding("unknown", "DNS-сервер вернул ошибку", dns.RcodeToString[rcode])
	}
	v := rrValues(rr)
	if len(v) == 0 {
		if optional {
			return finding("neutral", "Необязательная запись не найдена")
		}
		return finding("missing", "Запись не найдена")
	}
	return finding("ok", "Запись найдена", v...)
}
func siteDNS(ctx context.Context, host, domain string) map[string]Finding {
	out := map[string]Finding{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range []struct {
		k        string
		t        uint16
		optional bool
	}{{"a", dns.TypeA, false}, {"aaaa", dns.TypeAAAA, true}, {"cname", dns.TypeCNAME, true}, {"ns", dns.TypeNS, true}} {
		wg.Add(1)
		go func(k string, t uint16, opt bool) {
			defer wg.Done()
			f := dnsFinding(ctx, host, t, opt)
			mu.Lock()
			out[k] = f
			mu.Unlock()
		}(p.k, p.t, p.optional)
	}
	wg.Wait()
	out["delegation"] = delegation(ctx, host, domain)
	return out
}
func delegation(ctx context.Context, host, domain string) Finding {
	zone := domain
	parts := strings.Split(host, ".")
	for i := 0; i < len(parts)-strings.Count(domain, ".")-1; i++ {
		candidate := strings.Join(parts[i:], ".")
		rr, _, err := records(ctx, candidate, dns.TypeNS)
		if err == nil && len(rrValues(rr)) > 0 {
			zone = candidate
			break
		}
	}
	parent := strings.Join(strings.Split(zone, ".")[1:], ".")
	parentNS, _, err := records(ctx, parent, dns.TypeNS)
	if err != nil || len(parentNS) == 0 {
		return finding("unknown", "NS родительской зоны недоступны")
	}
	names := rrValues(parentNS)
	ips, err := resolvedPublic(ctx, names[0])
	if err != nil || len(ips) == 0 || !publicIP(ips[0]) {
		return finding("unknown", "Сервер родительской зоны недоступен")
	}
	delegated, _, err := query(ctx, zone, dns.TypeNS, ips[0].String())
	if err != nil {
		return finding("unknown", "Делегирование недоступно", err.Error())
	}
	p := rrValues(delegated)
	if len(p) == 0 {
		return finding("missing", "Делегирование не найдено", "Зона: "+zone)
	}
	authoritativeIPs, err := resolvedPublic(ctx, p[0])
	if err != nil || len(authoritativeIPs) == 0 {
		return finding("unknown", "Авторитетный DNS недоступен")
	}
	own, _, err := query(ctx, zone, dns.TypeNS, authoritativeIPs[0].String())
	if err != nil {
		return finding("unknown", "Авторитетные NS недоступны")
	}
	a := rrValues(own)
	return compareNS(zone, p, a)
}
func compareNS(zone string, p, a []string) Finding {
	if len(a) == 0 {
		return finding("unknown", "Авторитетный DNS не ответил NS")
	}
	if strings.Join(p, ",") != strings.Join(a, ",") {
		return finding("warning", "NS родителя и зоны различаются", append([]string{"Зона: " + zone, "Родитель: " + strings.Join(p, ", ")}, "Зона: "+strings.Join(a, ", "))...)
	}
	return finding("ok", "Делегирование согласовано", append([]string{"Зона: " + zone}, p...)...)
}
func siteTLS(ctx context.Context, host string) Finding {
	opCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := resolvedPublic(opCtx, host)
	if err != nil {
		return finding("unknown", "TLS не проверен", err.Error())
	}
	conn, err := dialPublic(opCtx, ips, "443")
	if err != nil {
		return finding("unknown", "Нет TLS-соединения", err.Error())
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	tc := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err := tc.HandshakeContext(opCtx); err != nil {
		return finding("warning", "Ошибка TLS или доверия сертификату", err.Error())
	}
	cert := tc.ConnectionState().PeerCertificates[0]
	f := finding("ok", "Сертификат действителен", "Издатель: "+cert.Issuer.CommonName, "Действует до: "+cert.NotAfter.UTC().Format(time.RFC3339))
	if time.Until(cert.NotAfter) < 30*24*time.Hour {
		f.Status = "warning"
		f.Summary = "Сертификат скоро истечёт"
	}
	return f
}

type lookupFn func(context.Context, string, uint16) ([]dns.RR, int, error)

func mailChecks(ctx context.Context, host, domain string) map[string]Finding {
	return mailChecksWith(ctx, host, domain, records)
}
func mailChecksWith(ctx context.Context, host, domain string, lookup lookupFn) map[string]Finding {
	out := map[string]Finding{}
	mx, rcode, err := lookup(ctx, host, dns.TypeMX)
	if err != nil {
		out["mx"] = finding("unknown", "MX не проверен", err.Error())
	} else if rcode != dns.RcodeSuccess {
		out["mx"] = finding("unknown", "DNS-сервер вернул ошибку", dns.RcodeToString[rcode])
	} else if len(mx) == 1 {
		if m, ok := mx[0].(*dns.MX); ok && m.Preference == 0 && m.Mx == "." {
			out["mx"] = finding("neutral", "Домен явно не принимает почту (null MX)", "0 .")
		} else {
			out["mx"] = finding("ok", "MX найден", rrValues(mx)...)
		}
	} else if len(mx) > 1 {
		out["mx"] = finding("ok", "MX найдены", rrValues(mx)...)
	} else {
		out["mx"] = finding("missing", "MX не найден")
	}
	out["spf"] = spfWith(ctx, host, lookup)
	out["dkim"] = dkimWith(ctx, host, lookup)
	out["dmarc"] = dmarcWith(ctx, host, domain, lookup)
	return out
}
func txtMatchingWith(ctx context.Context, name, prefix string, lookup lookupFn) ([]string, error) {
	rr, _, err := lookup(ctx, name, dns.TypeTXT)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, v := range rrValues(rr) {
		if strings.HasPrefix(strings.ToLower(v), prefix) {
			out = append(out, v)
		}
	}
	return out, nil
}
func spfWith(ctx context.Context, host string, lookup lookupFn) Finding {
	v, err := txtMatchingWith(ctx, host, "v=spf1", lookup)
	if err != nil {
		return finding("unknown", "SPF не проверен", err.Error())
	}
	if len(v) == 0 {
		return finding("missing", "SPF не найден")
	}
	if len(v) > 1 {
		return finding("warning", "Несколько SPF-записей: конфликт", v...)
	}
	tokens := strings.Fields(v[0])
	if len(tokens) < 2 {
		return finding("warning", "SPF выглядит неполным", v[0])
	}
	return finding("ok", "SPF найден", v[0])
}
func dkimWith(ctx context.Context, host string, lookup lookupFn) Finding {
	found := []string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, sel := range selectors {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			v, _ := txtMatchingWith(ctx, s+"._domainkey."+host, "v=dkim1", lookup)
			if len(v) > 0 {
				mu.Lock()
				found = append(found, s+": "+v[0])
				mu.Unlock()
			}
		}(sel)
	}
	wg.Wait()
	if len(found) == 0 {
		return finding("neutral", "Не найден среди проверенных селекторов", strings.Join(selectors, ", "))
	}
	sort.Strings(found)
	return finding("ok", "DKIM найден среди проверенных селекторов", found...)
}
func dmarcWith(ctx context.Context, host, domain string, lookup lookupFn) Finding {
	for _, name := range []string{host, domain} {
		v, err := txtMatchingWith(ctx, "_dmarc."+name, "v=dmarc1", lookup)
		if err != nil {
			return finding("unknown", "DMARC не проверен", err.Error())
		}
		if len(v) > 1 {
			return finding("warning", "Конфликт DMARC-записей", v...)
		}
		if len(v) == 1 {
			f := finding("ok", "DMARC найден", v[0])
			if name != host {
				f.Summary = "Наследуется DMARC организационного домена"
			}
			if !strings.Contains(strings.ToLower(v[0]), "p=") {
				f.Status = "warning"
				f.Summary = "DMARC без политики p="
			}
			return f
		}
		if name == domain {
			break
		}
	}
	return finding("missing", "DMARC не найден")
}
func (c *Checker) registration(ctx context.Context, domain string) Registration {
	now := time.Now().UTC()
	reg := Registration{Status: "unknown", Source: "RDAP", Detail: "Не удалось определить статус", CheckedAt: now}
	if strings.HasSuffix(domain, ".ru") || strings.HasSuffix(domain, ".xn--p1ai") {
		return whois(ctx, domain)
	}
	url := c.rdapURL(ctx, domain)
	if url == "" {
		reg.Detail = "Для этой зоны нет RDAP-сервера в IANA"
		return reg
	}
	code, _, _, err := requestPinned(ctx, url, "GET", 128*1024)
	if err != nil {
		reg.Detail = "RDAP недоступен: " + err.Error()
		return reg
	}
	return classifyRDAP(code, now)
}
func classifyRDAP(code int, now time.Time) Registration {
	reg := Registration{Status: "unknown", Source: "RDAP", CheckedAt: now}
	switch code {
	case 200:
		reg.Status = "registered"
		reg.Detail = "Регистрационная запись найдена"
	case 404:
		reg.Status = "unregistered"
		reg.Detail = "Регистрационная запись не найдена; доступность подтвердит регистратор"
	case 429:
		reg.Detail = "RDAP ограничил число запросов"
	default:
		reg.Detail = fmt.Sprintf("RDAP вернул HTTP %d", code)
	}
	return reg
}
func (c *Checker) rdapURL(ctx context.Context, domain string) string {
	c.bootstrapMu.Lock()
	defer c.bootstrapMu.Unlock()
	if time.Since(c.loaded) > 24*time.Hour {
		code, _, body, err := requestPinned(ctx, "https://data.iana.org/rdap/dns.json", "GET", 2*1024*1024)
		if err == nil && code == 200 {
			if m := bootstrapMap(body); len(m) > 0 {
				c.bootstrap = m
				c.loaded = time.Now()
			}
		}
	}
	tld := domain[strings.LastIndex(domain, ".")+1:]
	base := c.bootstrap[tld]
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/domain/" + domain
}
