package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed prices.json
var pricesJSON []byte

func catalogPrice(tld string) (float64, float64, string, string, bool) {
	var data struct {
		Source    string `json:"source"`
		CheckedAt string `json:"checkedAt"`
		Zones     map[string]struct {
			FirstYear float64 `json:"firstYear"`
			Renewal   float64 `json:"renewal"`
		} `json:"zones"`
	}
	if json.Unmarshal(pricesJSON, &data) != nil {
		return 0, 0, "", "", false
	}
	p, ok := data.Zones[tld]
	return p.FirstYear, p.Renewal, data.Source, data.CheckedAt, ok
}
func priceStale(date string, now time.Time) bool {
	d, err := time.Parse("2006-01-02", date)
	return err != nil || now.Sub(d) > 7*24*time.Hour
}

func whois(ctx context.Context, domain string) Registration {
	opCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r := Registration{Status: "unknown", Source: "WHOIS whois.tcinet.ru", Detail: "WHOIS недоступен", CheckedAt: time.Now().UTC()}
	ips, err := resolvedPublic(opCtx, "whois.tcinet.ru")
	if err != nil {
		r.Detail = err.Error()
		return r
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(opCtx, "tcp", net.JoinHostPort(ips[0].String(), "43"))
	if err != nil {
		r.Detail = err.Error()
		return r
	}
	defer conn.Close()
	if deadline, ok := opCtx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	fmt.Fprintf(conn, "%s\r\n", domain)
	b, err := io.ReadAll(io.LimitReader(conn, 65536))
	if err != nil {
		r.Detail = err.Error()
		return r
	}
	return classifyWHOIS(domain, string(b), r.CheckedAt)
}
func classifyWHOIS(domain, body string, checkedAt time.Time) Registration {
	r := Registration{Status: "unknown", Source: "WHOIS whois.tcinet.ru", CheckedAt: checkedAt}
	s := strings.ToLower(body)
	if strings.Contains(s, "no entries found") || strings.Contains(s, "not found") || strings.Contains(s, "no matching record") {
		r.Status = "unregistered"
		r.Detail = "Регистрационная запись не найдена; доступность подтвердит регистратор"
	} else if strings.Contains(s, "domain:") && strings.Contains(s, strings.ToLower(domain)) {
		r.Status = "registered"
		r.Detail = "Регистрационная запись найдена"
	} else {
		r.Detail = "WHOIS вернул неопределённый ответ"
	}
	return r
}
func registrarURL(name, domain string) string {
	key := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(name, ".", "_"), " ", "")) + "_URL"
	if v := os.Getenv(key); v != "" {
		return strings.ReplaceAll(v, "{domain}", url.QueryEscape(domain))
	}
	switch name {
	case "Porkbun":
		return "https://porkbun.com/checkout/search?q=" + url.QueryEscape(domain)
	case "Namecheap":
		return "https://www.namecheap.com/domains/registration/results/?domain=" + url.QueryEscape(domain)
	case "GoDaddy":
		return "https://www.godaddy.com/domainsearch/find?domainToCheck=" + url.QueryEscape(domain)
	case "REG.RU":
		return "https://www.reg.ru/domain/new/?dname=" + url.QueryEscape(domain)
	default:
		return "https://timeweb.com/ru/services/domains/"
	}
}

type zonePrice struct {
	FirstYear  float64 `json:"firstYear"`
	Renewal    float64 `json:"renewal"`
	Promo      bool    `json:"promo"`
	Conditions string  `json:"conditions"`
}
type registrarPrices struct {
	Source    string               `json:"source"`
	CheckedAt string               `json:"checkedAt"`
	Currency  string               `json:"currency"`
	Zones     map[string]zonePrice `json:"zones"`
}

func catalogOffer(name, domain, tld string) Offer {
	offer := Offer{Registrar: name, URL: registrarURL(name, domain), Kind: "catalog", Source: "Тариф для зоны не подтверждён", Currency: "USD"}
	if name == "REG.RU" || name == "Timeweb" {
		offer.Currency = "RUB"
	}
	var catalog struct {
		registrarPrices
		Other map[string]registrarPrices `json:"other"`
	}
	if json.Unmarshal(pricesJSON, &catalog) != nil {
		return offer
	}
	entry := catalog.registrarPrices
	if name != "Porkbun" {
		entry = catalog.Other[name]
	}
	if entry.Source != "" {
		offer.Source = entry.Source
		offer.SourceDate = entry.CheckedAt
	}
	if entry.Currency != "" {
		offer.Currency = entry.Currency
	}
	zone, ok := entry.Zones[tld]
	if !ok {
		return offer
	}
	offer.FirstYear = &zone.FirstYear
	offer.Renewal = &zone.Renewal
	offer.Kind = "from"
	offer.Promo = zone.Promo
	offer.Conditions = zone.Conditions
	offer.Stale = priceStale(entry.CheckedAt, time.Now().UTC())
	return offer
}
func markBestOffers(offers []Offer) []Offer {
	minimum := map[string]float64{}
	for i := range offers {
		offers[i].Badge = ""
		o := offers[i]
		if o.FirstYear == nil || o.Stale {
			continue
		}
		key := fmt.Sprintf("%s:%s:%t", o.Currency, o.Kind, o.Promo)
		if current, ok := minimum[key]; !ok || *o.FirstYear < current {
			minimum[key] = *o.FirstYear
		}
	}
	for i := range offers {
		o := &offers[i]
		if o.FirstYear == nil || o.Stale {
			continue
		}
		key := fmt.Sprintf("%s:%s:%t", o.Currency, o.Kind, o.Promo)
		if *o.FirstYear == minimum[key] {
			if o.Kind == "exact" {
				o.Badge = "Выгодно"
			} else {
				o.Badge = "Минимальный тариф от"
			}
		}
	}
	return offers
}
func offers(ctx context.Context, domain string) []Offer {
	tld := domain[strings.LastIndex(domain, ".")+1:]
	out := []Offer{}
	for _, name := range []string{"Porkbun", "Namecheap", "GoDaddy", "REG.RU", "Timeweb"} {
		out = append(out, catalogOffer(name, domain, tld))
	}
	date := time.Now().UTC().Format("2006-01-02")
	if os.Getenv("PORKBUN_API_KEY") != "" && os.Getenv("PORKBUN_SECRET_API_KEY") != "" {
		if body, err := requestPorkbun(ctx, domain); err == nil {
			var exact struct {
				Status   string `json:"status"`
				Response struct {
					Avail          string `json:"avail"`
					Price          string `json:"price"`
					FirstYearPromo string `json:"firstYearPromo"`
					Additional     struct {
						Renewal struct {
							Price string `json:"price"`
						} `json:"renewal"`
					} `json:"additional"`
				} `json:"response"`
			}
			if json.Unmarshal(body, &exact) == nil && strings.EqualFold(exact.Status, "SUCCESS") && exact.Response.Avail == "yes" {
				first, e1 := strconv.ParseFloat(exact.Response.Price, 64)
				renew, e2 := strconv.ParseFloat(exact.Response.Additional.Renewal.Price, 64)
				if e1 == nil {
					out[0].FirstYear = &first
					if e2 == nil {
						out[0].Renewal = &renew
					}
					out[0].Kind = "exact"
					out[0].Source = "https://api.porkbun.com/api/json/v3/domain/checkDomain/" + domain
					out[0].SourceDate = date
					out[0].Stale = false
					out[0].Promo = exact.Response.FirstYearPromo == "yes"
					if out[0].Promo {
						out[0].Conditions = "Акция на первый год"
					} else {
						out[0].Conditions = ""
					}
					return markBestOffers(out)
				}
			}
		}
	}
	code, _, body, err := requestPinned(ctx, "https://api.porkbun.com/api/json/v3/pricing/get", "GET", 2*1024*1024)
	if err == nil && code == 200 {
		var raw struct {
			Status  string `json:"status"`
			Pricing map[string]struct {
				Registration string `json:"registration"`
				Renewal      string `json:"renewal"`
			} `json:"pricing"`
		}
		if json.Unmarshal(body, &raw) == nil && strings.EqualFold(raw.Status, "SUCCESS") {
			if p, ok := raw.Pricing[tld]; ok {
				first, e1 := strconv.ParseFloat(p.Registration, 64)
				renew, e2 := strconv.ParseFloat(p.Renewal, 64)
				if e1 == nil && e2 == nil {
					out[0].FirstYear = &first
					out[0].Renewal = &renew
					out[0].Kind = "from"
					out[0].Source = "https://api.porkbun.com/api/json/v3/pricing/get"
					out[0].SourceDate = date
					out[0].Stale = false
					out[0].Promo = first < renew
					out[0].Conditions = ""
					if out[0].Promo {
						out[0].Conditions = "Тариф первого года ниже продления"
					}
				}
			}
		}
	}
	return markBestOffers(out)
}
