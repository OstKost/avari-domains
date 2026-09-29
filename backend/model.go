package main

import (
	"net"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

type Finding struct {
	Status    string    `json:"status"`
	Summary   string    `json:"summary"`
	Values    []string  `json:"values,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}
type Registration struct {
	Status    string    `json:"status"`
	Source    string    `json:"source"`
	Detail    string    `json:"detail"`
	CheckedAt time.Time `json:"checkedAt"`
}
type Offer struct {
	Registrar  string   `json:"registrar"`
	URL        string   `json:"url"`
	FirstYear  *float64 `json:"firstYear,omitempty"`
	Renewal    *float64 `json:"renewal,omitempty"`
	Currency   string   `json:"currency"`
	Kind       string   `json:"kind"`
	Source     string   `json:"source"`
	SourceDate string   `json:"sourceDate"`
	Stale      bool     `json:"stale"`
	Badge      string   `json:"badge,omitempty"`
	Promo      bool     `json:"promo"`
	Conditions string   `json:"conditions,omitempty"`
}
type Result struct {
	ID           string             `json:"id"`
	Input        string             `json:"input"`
	Hostname     string             `json:"hostname"`
	Domain       string             `json:"domain"`
	Registration Registration       `json:"registration"`
	Site         map[string]Finding `json:"site"`
	Mail         map[string]Finding `json:"mail"`
	Offers       []Offer            `json:"offers"`
	Preview      Preview            `json:"preview"`
	Robots       Finding            `json:"robots"`
	Sitemap      Finding            `json:"sitemap"`
	CheckedAt    time.Time          `json:"checkedAt"`
	Region       string             `json:"region"`
	Cached       bool               `json:"cached"`
}

type Preview struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image"`
	SiteName    string `json:"siteName"`
	URL         string `json:"url"`
	Status      string `json:"status"`
	Summary     string `json:"summary"`
}
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type inputValidationError struct {
	code    string
	message string
}

func (e inputValidationError) Error() string { return e.message }

func normalizeInput(input string) (string, string, error) {
	s := strings.TrimSpace(input)
	if s == "" || len(s) > 2048 {
		if s == "" {
			return "", "", inputValidationError{"input_required", "The input is empty"}
		}
		return "", "", inputValidationError{"input_too_long", "The input exceeds 2048 characters"}
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Hostname() == "" {
		return "", "", inputValidationError{"invalid_url", "Expected a domain or HTTP/HTTPS URL without credentials"}
	}
	if p := u.Port(); p != "" && p != "80" && p != "443" {
		return "", "", inputValidationError{"unsupported_port", "Non-standard ports are not supported"}
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if net.ParseIP(host) != nil || strings.Contains(host, ":") {
		return "", "", inputValidationError{"ip_not_supported", "IP addresses are not supported"}
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || len(ascii) > 253 || !strings.Contains(ascii, ".") {
		return "", "", inputValidationError{"invalid_domain", "The domain name is invalid"}
	}
	for _, label := range strings.Split(ascii, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", "", inputValidationError{"invalid_domain", "The domain name is invalid"}
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", "", inputValidationError{"invalid_domain", "The domain name is invalid"}
			}
		}
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(ascii)
	if err != nil {
		return "", "", inputValidationError{"registrable_domain_not_found", "Could not determine the registrable domain"}
	}
	return ascii, domain, nil
}
