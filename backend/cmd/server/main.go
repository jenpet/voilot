package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jenpet/voilot/internal/agent"
	"github.com/jenpet/voilot/internal/api"
	"github.com/jenpet/voilot/internal/config"
	"github.com/jenpet/voilot/internal/sessionmap"
	"github.com/jenpet/voilot/internal/stt"
	"github.com/jenpet/voilot/internal/tts"
	"github.com/jenpet/voilot/internal/workspace"
	"github.com/rs/cors"
)

// Set via -ldflags at build time.
var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	var (
		port         = flag.Int("port", 8080, "HTTP server port")
		hostname     = flag.String("hostname", "127.0.0.1", "Hostname to bind to")
		configPath   = flag.String("config", "", "Path to config file (default: ~/.config/voilot/config.json)")
		dataDir      = flag.String("data-dir", "voilot-data", "Directory for persistent data (session map, PID files, etc.)")
		allowOrigins = flag.String("cors-origins", "", "Allowed CORS origins (comma-separated, empty = deny cross-origin)")
		// CLI overrides for deployment flexibility (Docker compose, etc.)
		cliTTSUrl       = flag.String("tts-url", "", "Override TTS server URL from config")
		cliSTTUrl       = flag.String("stt-url", "", "Override STT server URL from config")
		cliWorkspaceDir = flag.String("workspace-dir", "", "Override workspace directory from config")
		logLevel        = flag.String("log-level", "info", "Log level: debug, info, warn, error")
		logFormat       = flag.String("log-format", "text", "Log format: text, json")
	)
	flag.Parse()

	// Initialize structured logging
	initLogging(*logLevel, *logFormat)

	// Verify required binaries are available
	checkRequiredBinaries()

	// Load config
	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = config.DefaultPath()
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("Configuration error", "error", err)
		os.Exit(1)
	}
	slog.Info("Config loaded", "path", cfgPath)

	// Apply CLI overrides
	if *cliTTSUrl != "" {
		cfg.TTSUrl = *cliTTSUrl
	}
	if *cliSTTUrl != "" {
		cfg.STTUrl = *cliSTTUrl
	}
	if *cliWorkspaceDir != "" {
		cfg.Workspace = *cliWorkspaceDir
	}

	// Build providers from config
	providers := buildProviders(cfg)
	if len(providers) == 0 {
		slog.Error("No supported providers configured")
		os.Exit(1)
	}

	pidDir := fmt.Sprintf("%s/pids", *dataDir)
	registry, err := agent.NewProviderRegistry(providers, cfg.DefaultProvider, pidDir,
		agent.WithMaxInstances(cfg.MaxInstances),
		agent.WithIdleTimeout(cfg.IdleTimeoutDuration()),
	)
	if err != nil {
		slog.Error("Failed to create provider registry", "error", err)
		os.Exit(1)
	}
	defer registry.Close()
	slog.Info("Provider registry initialized", "providers", len(providers), "maxInstances", cfg.MaxInstances, "pidDir", pidDir)

	// Start config file watcher for hot-reload
	watchCtx, watchCancel := context.WithCancel(context.Background())
	defer watchCancel()
	watcher := config.NewWatcher(cfgPath, 2*time.Second)
	go watcher.Start(watchCtx)
	go func() {
		for change := range watcher.Changes() {
			// Check for non-reloadable field changes
			if change.Old != nil && change.Old.Workspace != change.New.Workspace {
				slog.Warn("workspace changed in config, will take effect after restart",
					"old", change.Old.Workspace, "new", change.New.Workspace)
			}
			newProviders := buildProviders(change.New)
			registry.ReloadProviders(newProviders, change.New.DefaultProvider,
				agent.WithMaxInstances(change.New.MaxInstances),
				agent.WithIdleTimeout(change.New.IdleTimeoutDuration()),
			)
			// Update TTS/STT URLs for next request (handled via server reference if needed)
			if change.Old != nil && change.Old.TTSUrl != change.New.TTSUrl {
				slog.Info("ttsUrl updated", "url", change.New.TTSUrl)
			}
			if change.Old != nil && change.Old.STTUrl != change.New.STTUrl {
				slog.Info("sttUrl updated", "url", change.New.STTUrl)
			}
		}
	}()

	// Initialize TTS provider (optional)
	var ttsProvider tts.Provider
	if cfg.TTSUrl != "" {
		ttsProvider = tts.NewKokoroProvider(cfg.TTSUrl)
		slog.Info("TTS enabled", "provider", "kokoro", "url", cfg.TTSUrl)
	} else {
		slog.Info("TTS disabled (not configured)")
	}

	// Initialize STT provider (optional)
	var sttProvider stt.Provider
	if cfg.STTUrl != "" {
		sttProvider = stt.NewWhisperProvider(cfg.STTUrl)
		slog.Info("STT enabled", "provider", "faster-whisper", "url", cfg.STTUrl)
	} else {
		slog.Info("STT disabled (not configured)")
	}

	// Initialize workspace scanner
	var scanner *workspace.Scanner
	if cfg.Workspace != "" {
		scanner = workspace.NewScanner(cfg.Workspace)
		if _, err := scanner.Scan(); err != nil {
			slog.Error("Failed to scan workspace", "error", err)
			os.Exit(1)
		}
		slog.Info("Workspace enabled", "path", cfg.Workspace)
	} else {
		slog.Info("Workspace disabled (not configured)")
	}

	// Session map
	sesMap, err := sessionmap.New(fmt.Sprintf("%s/session-map.json", *dataDir))
	if err != nil {
		slog.Error("Failed to load session map", "error", err)
		os.Exit(1)
	}
	slog.Info("Session map loaded", "path", fmt.Sprintf("%s/session-map.json", *dataDir))

	// Worktree defaults
	wtDefaults, err := agent.NewWorktreeDefaults(fmt.Sprintf("%s/worktree-defaults.json", *dataDir))
	if err != nil {
		slog.Error("Failed to load worktree defaults", "error", err)
		os.Exit(1)
	}

	// Create API server
	server := api.NewServer(registry, ttsProvider, sttProvider, scanner, sesMap, wtDefaults, api.BuildInfo{
		Version:   version,
		BuildTime: buildTime,
	})

	// CORS middleware
	origins := []string{}
	if *allowOrigins != "" {
		origins = strings.Split(*allowOrigins, ",")
	}
	handler := cors.New(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	}).Handler(server)

	// Set up HTTP server with graceful shutdown
	addr := fmt.Sprintf("%s:%d", *hostname, *port)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Listen for shutdown signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("voilot backend starting",
			"addr", addr,
			"providers", len(providers),
			"tts", cfg.TTSUrl != "",
			"stt", cfg.STTUrl != "",
			"workspace", cfg.Workspace != "",
		)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Block until shutdown signal
	sig := <-stop
	slog.Info("Shutdown signal received, draining connections...", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("Shutdown error, forcing exit", "error", err)
		os.Exit(1)
	}

	slog.Info("Server stopped gracefully")
}

func initLogging(level, format string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Bridge stdlib log to slog for third-party libraries
	log.SetFlags(0)
	log.SetOutput(&slogWriter{logger: logger})
}

// slogWriter bridges stdlib log.Printf calls to slog.
type slogWriter struct {
	logger *slog.Logger
}

func (w *slogWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimRight(string(p), "\n")
	w.logger.Info(msg, "source", "stdlib")
	return len(p), nil
}

// buildProviders creates agent.Provider instances from the config's provider map.
func buildProviders(cfg *config.Config) map[string]agent.Provider {
	providers := make(map[string]agent.Provider)
	for name, pc := range cfg.Providers {
		switch pc.Type {
		case "opencode":
			providers[name] = agent.NewOpenCodeProvider(pc.Binary, pc.Env)
		default:
			slog.Warn("Unsupported provider type, skipping", "provider", name, "type", pc.Type)
		}
	}
	return providers
}

// requiredBinaries lists external commands that must be on $PATH for the
// backend to function. Extend this slice when new dependencies are added.
var requiredBinaries = []string{"git", "wt"}

// checkRequiredBinaries verifies all required binaries are available and
// exits with an error if any are missing.
func checkRequiredBinaries() {
	var missing []string
	for _, bin := range requiredBinaries {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		slog.Error("Required binaries not found on $PATH", "missing", missing)
		os.Exit(1)
	}
}
