package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/jenpet/voilot/internal/agent"
	"github.com/jenpet/voilot/internal/api"
	"github.com/jenpet/voilot/internal/config"
	"github.com/jenpet/voilot/internal/sessionmap"
	"github.com/jenpet/voilot/internal/stt"
	"github.com/jenpet/voilot/internal/tts"
	"github.com/jenpet/voilot/internal/workspace"
	"github.com/rs/cors"
)

func main() {
	var (
		port         = flag.Int("port", 8080, "HTTP server port")
		hostname     = flag.String("hostname", "127.0.0.1", "Hostname to bind to")
		configPath   = flag.String("config", "", "Path to config file (default: ~/.config/voilot/config.json)")
		dataDir      = flag.String("data-dir", "voilot-data", "Directory for persistent data (session map, PID files, etc.)")
		allowOrigins = flag.String("cors-origins", "*", "Allowed CORS origins (comma-separated)")
		// CLI overrides for deployment flexibility (Docker compose, etc.)
		cliTTSUrl       = flag.String("tts-url", "", "Override TTS server URL from config")
		cliSTTUrl       = flag.String("stt-url", "", "Override STT server URL from config")
		cliWorkspaceDir = flag.String("workspace-dir", "", "Override workspace directory from config")
	)
	flag.Parse()

	// Load config
	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = config.DefaultPath()
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}
	log.Printf("Config loaded from %s", cfgPath)

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
	providers := make(map[string]agent.Provider)
	for name, pc := range cfg.Providers {
		switch pc.Type {
		case "opencode":
			providers[name] = agent.NewOpenCodeProvider(pc.Binary)
		default:
			log.Printf("Warning: provider %q has unsupported type %q, skipping", name, pc.Type)
		}
	}
	if len(providers) == 0 {
		log.Fatalf("No supported providers configured")
	}

	pidDir := fmt.Sprintf("%s/pids", *dataDir)
	registry, err := agent.NewProviderRegistry(providers, cfg.DefaultProvider, pidDir,
		agent.WithMaxInstances(cfg.MaxInstances),
		agent.WithIdleTimeout(cfg.IdleTimeoutDuration()),
	)
	if err != nil {
		log.Fatalf("Failed to create provider registry: %v", err)
	}
	defer registry.Close()
	log.Printf("Provider registry: %d provider(s), max %d instances, PID dir %s", len(providers), cfg.MaxInstances, pidDir)

	// Initialize TTS provider (optional)
	var ttsProvider tts.Provider
	if cfg.TTSUrl != "" {
		ttsProvider = tts.NewKokoroProvider(cfg.TTSUrl)
		log.Printf("TTS enabled: Kokoro at %s", cfg.TTSUrl)
	} else {
		log.Println("TTS disabled (not configured)")
	}

	// Initialize STT provider (optional)
	var sttProvider stt.Provider
	if cfg.STTUrl != "" {
		sttProvider = stt.NewWhisperProvider(cfg.STTUrl)
		log.Printf("STT enabled: faster-whisper at %s", cfg.STTUrl)
	} else {
		log.Println("STT disabled (not configured)")
	}

	// Initialize workspace scanner
	var scanner *workspace.Scanner
	if cfg.Workspace != "" {
		scanner = workspace.NewScanner(cfg.Workspace)
		if _, err := scanner.Scan(); err != nil {
			log.Fatalf("Failed to scan workspace: %v", err)
		}
		log.Printf("Workspace enabled: %s", cfg.Workspace)
	} else {
		log.Println("Workspace disabled (not configured)")
	}

	// Session map
	sesMap, err := sessionmap.New(fmt.Sprintf("%s/session-map.json", *dataDir))
	if err != nil {
		log.Fatalf("Failed to load session map: %v", err)
	}
	log.Printf("Session map: %s/session-map.json", *dataDir)

	// Worktree defaults
	wtDefaults, err := agent.NewWorktreeDefaults(fmt.Sprintf("%s/worktree-defaults.json", *dataDir))
	if err != nil {
		log.Fatalf("Failed to load worktree defaults: %v", err)
	}

	// Create API server
	server := api.NewServer(registry, ttsProvider, sttProvider, scanner, sesMap, wtDefaults)

	// CORS middleware
	handler := cors.New(cors.Options{
		AllowedOrigins:   []string{*allowOrigins},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	}).Handler(server)

	addr := fmt.Sprintf("%s:%d", *hostname, *port)
	log.Printf("voilot backend starting on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
