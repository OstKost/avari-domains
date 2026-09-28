package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type bucket struct {
	tokens float64
	at     time.Time
}
type server struct {
	store   *Store
	checker *Checker
	work    chan struct{}
	group   singleflight.Group
	mu      sync.Mutex
	limits  map[string]bucket
	trusted map[string]bool
}

func main() {
	dbpath := env("SQLITE_PATH", "data/avari.sqlite")
	if err := os.MkdirAll(filepath.Dir(dbpath), 0755); err != nil {
		log.Fatal(err)
	}
	store, err := openStore(dbpath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.db.Close()
	s := &server{store: store, checker: newChecker(), work: make(chan struct{}, 8), limits: map[string]bucket{}, trusted: map[string]bool{}}
	for _, ip := range strings.Split(os.Getenv("TRUSTED_PROXY_IPS"), ",") {
		s.trusted[strings.TrimSpace(ip)] = true
	}
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for range t.C {
			store.cleanup()
		}
	}()
	store.cleanup()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /api/check", s.check)
	mux.HandleFunc("GET /api/history", s.history)
	mux.HandleFunc("GET /api/history/{id}", s.historyItem)
	mux.HandleFunc("GET /api/stats", s.stats)
	static := env("STATIC_DIR", "../frontend/dist")
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			apiError(w, 404, "not_found", "Маршрут не найден")
			return
		}
		p := filepath.Clean(r.URL.Path)
		if p == "/" {
			p = "/index.html"
		}
		file := filepath.Join(static, p)
		if !strings.HasPrefix(file, filepath.Clean(static)+string(os.PathSeparator)) {
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(file); err != nil {
			file = filepath.Join(static, "index.html")
		}
		http.ServeFile(w, r, file)
	}))
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		tracked := &statusWriter{ResponseWriter: w, status: 200}
		mux.ServeHTTP(tracked, r)
		log.Printf("method=%s path=%s status=%d duration=%s", r.Method, r.URL.Path, tracked.status, time.Since(start))
	})
	log.Printf("listening on %s, region=%s", env("LISTEN_ADDR", ":8080"), env("CHECK_REGION", "не указан"))
	log.Fatal(http.ListenAndServe(env("LISTEN_ADDR", ":8080"), h))
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, status int, code, msg string) {
	log.Printf("api_error code=%s status=%d", code, status)
	respond(w, status, APIError{code, msg})
}
func (s *server) session(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie("avari_session"); err == nil && len(c.Value) == 48 {
		if _, err := hex.DecodeString(c.Value); err == nil {
			return c.Value
		}
	}
	id := randomID()
	remote, _, _ := net.SplitHostPort(r.RemoteAddr)
	secure := r.TLS != nil || s.trusted[remote] && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{Name: "avari_session", Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure, MaxAge: 90 * 86400})
	return id
}
func (s *server) clientIP(r *http.Request) string {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if s.trusted[ip] {
		if x := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); net.ParseIP(x) != nil {
			return x
		}
	}
	return ip
}
func (s *server) allow(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.limits[ip]
	now := time.Now()
	if !ok {
		b = bucket{tokens: 10, at: now}
	}
	b.tokens += now.Sub(b.at).Seconds() / 3
	if b.tokens > 10 {
		b.tokens = 10
	}
	b.at = now
	if b.tokens < 1 {
		s.limits[ip] = b
		return false
	}
	b.tokens--
	s.limits[ip] = b
	return true
}
func (s *server) check(w http.ResponseWriter, r *http.Request) {
	session := s.session(w, r)
	if !s.allow(s.clientIP(r)) {
		w.Header().Set("Retry-After", "3")
		apiError(w, 429, "rate_limited", "Слишком много запросов. Повторите через несколько секунд")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		Input string `json:"input"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		apiError(w, 400, "invalid_json", "Некорректный JSON")
		return
	}
	host, domain, err := normalizeInput(req.Input)
	if err != nil {
		apiError(w, 400, "invalid_input", err.Error())
		return
	}
	var result Result
	if cached, ok := s.store.cached(host); ok {
		result = *cached
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		v, err, _ := s.group.Do(host, func() (any, error) {
			if c, ok := s.store.cached(host); ok {
				return *c, nil
			}
			select {
			case s.work <- struct{}{}:
				defer func() { <-s.work }()
			case <-ctx.Done():
				return nil, errBusy
			}
			res := s.checker.run(ctx, req.Input, host, domain)
			ttl := 5 * time.Minute
			if res.Registration.Status == "unregistered" || res.Registration.Status == "available" {
				ttl = time.Minute
			}
			if res.Registration.Status == "unknown" {
				ttl = 30 * time.Second
			}
			if err := s.store.putCache(res, ttl); err != nil {
				log.Printf("cache_error=%v", err)
			}
			return res, nil
		})
		if err != nil {
			apiError(w, 503, "busy", "Проверка сейчас недоступна. Повторите позже")
			return
		}
		result = v.(Result)
	}
	result.ID = randomID()
	result.Input = req.Input
	result.Region = env("CHECK_REGION", "не указан")
	if err := s.store.save(session, result); err != nil {
		log.Printf("history_error=%v", err)
	}
	respond(w, 200, result)
}
func (s *server) history(w http.ResponseWriter, r *http.Request) {
	session := s.session(w, r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 0 || page > 10000 {
		apiError(w, 400, "invalid_page", "Некорректная страница")
		return
	}
	items, err := s.store.history(session, page)
	if err != nil {
		apiError(w, 500, "storage_error", "Не удалось загрузить историю")
		return
	}
	respond(w, 200, map[string]any{"items": items, "page": page, "pageSize": 20})
}
func (s *server) historyItem(w http.ResponseWriter, r *http.Request) {
	session := s.session(w, r)
	item, err := s.store.item(session, r.PathValue("id"))
	if err != nil {
		apiError(w, 404, "not_found", "Результат не найден")
		return
	}
	respond(w, 200, item)
}
func (s *server) stats(w http.ResponseWriter, r *http.Request) {
	session := s.session(w, r)
	v, err := s.store.stats(session)
	if err != nil {
		apiError(w, 500, "storage_error", "Не удалось загрузить статистику")
		return
	}
	respond(w, 200, v)
}
