package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/alecthomas/kong"
	htx "github.com/jonasohland/ext/http"
	slx "github.com/jonasohland/slog-ext/pkg/slog-ext"
)

type Options struct {
	Target   map[string]string `help:"Set scraping targets, can be repeated, examples: pgy0=http://10.130.200.80"`
	Listen   string            `help:"Listen address" default:"0.0.0.0:5066"`
	LogLevel string            `enum:"debug,info,warn,error" default:"info" help:"Set the log level"`
}

func logLevel(level string) slog.Leveler {
	switch level {
	case "trace":
		return slx.LevelTrace
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "silent":
		return slog.LevelError + 1
	}

	return slog.LevelInfo
}

type Handler struct {
	Targets map[string]string
	Mux     *http.ServeMux
}

func scrapeTarget(wg *sync.WaitGroup, url string, result chan<- any) {
	defer wg.Done()

	slog.Info("scrape", "target", url)
}

func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	var targets []string

	if target := r.URL.Query().Get("target"); target != "" {
		targetURL, ok := h.Targets[target]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("target not found"))

			return
		}

		targets = append(targets, targetURL)
	} else {
		for _, targetURL := range h.Targets {
			targets = append(targets, targetURL)
		}
	}

	var wg sync.WaitGroup

	results := make(chan any)

	wg.Add(len(targets))

	for _, targetURL := range targets {
		go scrapeTarget(&wg, targetURL, results)
	}

	wg.Wait()

	w.WriteHeader(http.StatusOK)
}

func NewHandler(targets map[string]string) *Handler {
	handler := &Handler{
		Targets: targets,
		Mux:     http.NewServeMux(),
	}

	handler.Mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) { handler.metrics(w, r) })

	return handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Mux.ServeHTTP(w, r)
}

func main() {
	var opts Options

	kong.Parse(&opts)

	ctx, cancel := signal.NotifyContext(context.Background())
	defer cancel()

	slog.SetDefault(slog.New(slx.NewHandler(os.Stderr, logLevel(opts.LogLevel))))

	srv, err := htx.NewContextServer(ctx, NewHandler(opts.Target), "tcp", opts.Listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		defer os.Exit(1)

		return
	}

	<-ctx.Done()

	if err := srv.Wait(time.Second); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		defer os.Exit(1)

		return
	}
}
