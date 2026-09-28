package main

import (
	"errors"
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
	CheckedAt    time.Time          `json:"checkedAt"`
	Region       string             `json:"region"`
	Cached       bool               `json:"cached"`
}
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func normalizeInput(input string) (string, string, error) {
	s := strings.TrimSpace(input)
	if s == "" || len(s) > 2048 {
		return "", "", errors.New("Введите домен или URL длиной до 2048 символов")
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Hostname() == "" {
		return "", "", errors.New("Введите домен или HTTP/HTTPS URL без учётных данных")
	}
	if p := u.Port(); p != "" && p != "80" && p != "443" {
		return "", "", errors.New("Нестандартный порт не поддерживается")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if net.ParseIP(host) != nil || strings.Contains(host, ":") {
		return "", "", errors.New("IP-адреса не поддерживаются")
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || len(ascii) > 253 || !strings.Contains(ascii, ".") {
		return "", "", errors.New("Некорректное доменное имя")
	}
	for _, label := range strings.Split(ascii, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", "", errors.New("Некорректное доменное имя")
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", "", errors.New("Некорректное доменное имя")
			}
		}
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(ascii)
	if err != nil {
		return "", "", errors.New("Не удалось определить регистрируемый домен")
	}
	return ascii, domain, nil
}
