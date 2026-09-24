// SPDX-License-Identifier: Apache-2.0

// skillsd serves an anonymous, publisher-reviewed skill catalog.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ajent/internal/skillcatalog"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", "127.0.0.1:8091", "HTTP bind address")
	origin := flag.String("origin", "http://127.0.0.1:8091", "Public HTTPS origin (loopback HTTP permitted)")
	manifest := flag.String("catalog", "", "Explicitly reviewed catalog JSON; default: bundled seed catalog")
	export := flag.Bool("export", false, "Write validated public edge snapshot to stdout and exit")
	flag.Parse()
	var reader = strings.NewReader(skillcatalog.SeedCatalog)
	var handler *skillcatalog.Catalog
	var err error
	if *manifest != "" {
		file, openErr := os.Open(*manifest)
		if openErr != nil {
			return fmt.Errorf("open configured catalog: %w", openErr)
		}
		handler, err = skillcatalog.New(file, *origin, skillcatalog.Bootstrap)
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	} else {
		handler, err = skillcatalog.New(reader, *origin, skillcatalog.Bootstrap)
	}
	if err != nil {
		return err
	}
	if *export {
		return handler.Export(os.Stdout)
	}
	server := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		return fmt.Errorf("serve skills: %w", err)
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return fmt.Errorf("stop skills: %w", err)
		}
		if err := <-done; err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}
