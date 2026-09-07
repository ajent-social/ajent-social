package main

import (
	"ajent/internal/api"
	"ajent/internal/bootstrap"
	"ajent/internal/jobs"
	"ajent/internal/store"
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
	"strings"
	"syscall"
	"time"
)

func main() {
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
	srv := &http.Server{Addr: *addr, Handler: headers(limit(m, trusted)), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
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
func headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := &responseStatus{ResponseWriter: w}
		w = response
		requestID := uuid.NewString()
		started := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("request panic", "request_id", requestID)
				if response.status == 0 {
					http.Error(w, "Internal server error.", 500)
				}
			}
			slog.Info("request", "request_id", requestID, "method", r.Method, "route", r.Pattern, "status", response.status, "duration_ms", time.Since(started).Milliseconds())
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
	status int
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
