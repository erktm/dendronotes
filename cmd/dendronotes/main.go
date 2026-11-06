// dendronotes – self-hosted Markdown notes.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"dendronotes/internal/config"
	"dendronotes/internal/server"
	"dendronotes/web"
)

// version is set at build time via -ldflags "-X main.version=…".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "serve":
		return serve(getenv, stdout)
	case "hash-password":
		return hashPassword(stdin, stdout, stderr)
	case "healthcheck":
		return healthcheck(getenv, stderr)
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version)
		return 0
	case "help", "--help", "-h":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", cmd)
		usage(stderr)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `dendronotes – self-hosted Markdown notes

Commands:
  serve           Start the server (default)
  hash-password   Read a password from the terminal or stdin and print an
                  argon2id hash for DENDRONOTES_ADMIN_PASSWORD_HASH
  healthcheck     Query /healthz of the running server (for Docker HEALTHCHECK)
  version         Print the version

Configuration via DENDRONOTES_* environment variables, see README.md.
`)
}

func serve(getenv func(string) string, stdout io.Writer) int {
	cfg, err := config.Load(getenv)
	log := newLogger(stdout, cfg.LogFormat, cfg.LogLevel)
	slog.SetDefault(log)
	if err != nil {
		log.Error("invalid configuration", "err", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	srv, err := server.New(cfg, log, web.Assets(), version)
	if err != nil {
		log.Error("startup failed", "err", err)
		return 1
	}
	if err := srv.Run(ctx); err != nil {
		log.Error("server stopped with error", "err", err)
		return 1
	}
	return 0
}

func newLogger(w io.Writer, format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if format == "text" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

func healthcheck(getenv func(string) string, stderr io.Writer) int {
	port := config.DefaultPort
	if v := getenv(config.EnvHTTPPort); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			fmt.Fprintf(stderr, "invalid %s: %q\n", config.EnvHTTPPort, v)
			return 1
		}
		port = p
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "healthz: HTTP %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
