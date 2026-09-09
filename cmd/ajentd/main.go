package main

import (
	"ajent/internal/api"
	"ajent/internal/bootstrap"
	"ajent/internal/jobs"
	"ajent/internal/store"
	"ajent/internal/telemetry"
	"ajent/internal/web"
	"ajent/migrations"
	"context"
	"flag"
	"github.com/google/uuid"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	loadChatEnv(".env")
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	demo := flag.Bool("seed-demo", false, "seed explicitly labelled sample community")
	healthcheck := flag.Bool("healthcheck", false, "check the local server and exit")
	migrateOnly := flag.Bool("migrate", false, "apply migrations and exit")
	flag.Parse()
	if *healthcheck {
		client := &http.Client{Timeout: 3 * time.Second}
		response, err := client.Get("http://127.0.0.1:8080/healthz")
		if err != nil {
			log.Fatal("health check failed")
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			log.Fatal("health check failed")
		}
		return
	}
	dsn := os.Getenv("AJENT_DSN")
	setup := os.Getenv("AJENT_SETUP_TOKEN")
	if dsn == "" {
		log.Fatal("AJENT_DSN is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, e := store.New(ctx, dsn)
	if e != nil {
		log.Fatal(e)
	}
	defer db.Pool.Close()
	if e = migrations.Apply(ctx, db.Pool); e != nil {
		log.Fatal(e)
	}
	if *migrateOnly {
		return
	}
	if len(setup) < 32 || strings.HasPrefix(setup, "replace-") {
		log.Fatal("AJENT_SETUP_TOKEN must be at least 32 characters; generate a random owner key")
	}
	origin := os.Getenv("AJENT_ORIGIN")
	if origin == "" {
		origin = "http://localhost:8080"
	}
	u, e := url.Parse(origin)
	if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.User != nil {
		log.Fatal("AJENT_ORIGIN must be an http(s) origin without a path")
	}
	secure := u.Scheme == "https"
	if !secure && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && os.Getenv("AJENT_ALLOW_INSECURE_DEV") != "1" {
		log.Fatal("Use HTTPS or explicitly enable insecure local development")
	}
	api.SetSetupToken(setup)
	owner, e := bootstrap.Ensure(ctx, db.Pool, "Your workspace")
	if e != nil {
		log.Fatal(e)
	}
	if *demo {
		if e = bootstrap.Demo(ctx, db.Pool, owner); e != nil {
			log.Fatal(e)
		}
	}
	a := api.New(db.Pool)
	trusted, e := parseTrustedProxies(os.Getenv("AJENT_TRUSTED_PROXIES"))
	if e != nil {
		log.Fatal(e)
	}
	w, e := web.New(a, secure, origin, setup, *demo)
	if e != nil {
		log.Fatal(e)
	}
	// Keep the previous HTTPS origin usable while installed agents migrate.
	legacy := os.Getenv("AJENT_LEGACY_ORIGIN")
	if legacy != "" {
		old, err := url.Parse(legacy)
		if err != nil || old.Scheme != "https" || old.Host == "" || old.User != nil || old.Path != "" || old.RawQuery != "" || old.ForceQuery || old.Fragment != "" || !secure {
			log.Fatal("AJENT_LEGACY_ORIGIN must be an HTTPS origin without a path")
		}
		w.LegacyOrigin = legacy
	}
	m := http.NewServeMux()
	a.Routes(m)
	w.Routes(m)
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if db.Pool.Ping(ctx) != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		w.Write([]byte("ok"))
	})
	jobsDone := jobs.Start(ctx, db.Pool)
	srv := &http.Server{Addr: *addr, Handler: headers(limit(m, trusted), m), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := srv.Shutdown(c); err != nil {
			slog.Error("graceful shutdown timed out", "error", err)
			if err := srv.Close(); err != nil {
				slog.Error("closing server", "error", err)
			}
		}
	}()
	slog.Info("ajent listening", "addr", *addr)
	err := srv.ListenAndServe()
	stop()
	<-shutdownDone
	<-jobsDone
	if err != nil && err != http.ErrServerClosed {
		db.Pool.Close()
		log.Fatal("HTTP listener failed: ", err)
	}

}
func headers(next http.Handler, routes *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := &responseStatus{ResponseWriter: w}
		w = response
		requestID := uuid.NewString()
		started := time.Now()
		// Resolve before downstream middleware clones the request or rejects it.
		// Never fall back to URL.Path: it can contain identifiers and secrets.
		_, route := routes.Handler(r)
		// All Ajent registrations include a method. ServeMux can return a raw
		// redirect path for CONNECT requests; that is not a route template.
		if !strings.Contains(route, " ") {
			route = "unmatched"
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		defer func() {
			if recovered := recover(); recovered != nil {
				response.outcome.ErrorCode = "panic"
				if response.status == 0 {
					http.Error(w, "Internal server error.", 500)
				}
			}
			if response.status == 0 {
				response.status = http.StatusOK
			}
			if response.outcome.ErrorCode == "" && response.status >= 400 {
				response.outcome.ErrorCode = "http_" + strconv.Itoa(response.status)
			}
			attrs := []any{"schema_version", 1, "request_id", requestID, "method", requestMethod(r.Method), "route", route, "status", response.status, "duration_ms", time.Since(started).Milliseconds(), "release", releaseRevision, "traffic", requestTraffic(route, r), "client", clientClass(r.Header.Get("X-Ajent-Client"))}
			if response.outcome.ErrorCode != "" {
				attrs = append(attrs, "error_code", response.outcome.ErrorCode)
			}
			if response.outcome.ResultCount != nil {
				attrs = append(attrs, "result_count", *response.outcome.ResultCount)
			}
			if response.outcome.PostKind != "" {
				attrs = append(attrs, "post_kind", response.outcome.PostKind, "reply", response.outcome.Reply, "structured", response.outcome.Structured)
			}
			if response.outcome.Replayed {
				attrs = append(attrs, "replayed", true)
			}
			slog.Info("request", attrs...)
		}()
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'")
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		next.ServeHTTP(w, r)
	})
}

type responseStatus struct {
	http.ResponseWriter
	status  int
	outcome telemetry.Outcome
}

func (w *responseStatus) RecordOutcome(outcome telemetry.Outcome) { w.outcome = outcome }

// buildRevision is set by release container builds, which exclude .git.
var buildRevision string

var releaseRevision = func() string {
	if buildRevision != "" {
		return buildRevision
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}
	return "unknown"
}()

func requestMethod(method string) string {
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
		return method
	default:
		return "other"
	}
}

func clientClass(value string) string {
	switch value {
	case "python-cli", "mcp", "probe":
		return value
	default:
		return "unknown"
	}
}

func trafficClass(route string) string {
	switch route {
	case "GET /healthz":
		return "health"
	case "GET /static/":
		return "asset"
	default:
		return "application"
	}
}

func (w *responseStatus) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
		w.ResponseWriter.WriteHeader(code)
	}
}
func (w *responseStatus) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
func (w *responseStatus) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func requestTraffic(route string, r *http.Request) string {
	if trafficClass(route) == "application" && clientClass(r.Header.Get("X-Ajent-Client")) == "probe" {
		return "probe"
	}
	return trafficClass(route)
}
