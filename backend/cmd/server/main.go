package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/jenpet/voilot/internal/agent"
	"github.com/jenpet/voilot/internal/api"
	"github.com/jenpet/voilot/internal/sessionmap"
	"github.com/jenpet/voilot/internal/stt"
	"github.com/jenpet/voilot/internal/tts"
	"github.com/jenpet/voilot/internal/workspace"
	"github.com/rs/cors"
)

func main() {
	var (
		port           = flag.Int("port", 8080, "HTTP server port")
		hostname       = flag.String("hostname", "127.0.0.1", "Hostname to bind to")
		opencodeBinary = flag.String("opencode-binary", "", "Path to opencode binary (default: resolve from PATH)")
		ttsURL         = flag.String("tts-url", "", "TTS server URL (optional)")
		sttURL         = flag.String("stt-url", "", "faster-whisper server URL (optional)")
		workspaceDir   = flag.String("workspace-dir", "", "Workspace directory for project/worktree discovery (optional)")
		dataDir        = flag.String("data-dir", "voilot-data", "Directory for persistent data (session map, PID files, etc.)")
		allowOrigins   = flag.String("cors-origins", "*", "Allowed CORS origins (comma-separated)")
	)
	flag.Parse()

	// Initialize provider registry (replaces single agentAdapter)
	provider := agent.NewOpenCodeProvider(*opencodeBinary)

	maxInstances := agent.DefaultMaxInstances
	if v := os.Getenv("VOILOT_MAX_INSTANCES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxInstances = n
		}
	}

	pidDir := fmt.Sprintf("%s/pids", *dataDir)
	registry, err := agent.NewProviderRegistry(provider, pidDir,
		agent.WithMaxInstances(maxInstances),
	)
	if err != nil {
		log.Fatalf("Failed to create provider registry: %v", err)
	}
	defer registry.Close()
	log.Printf("Provider registry: max %d instances, PID dir %s", maxInstances, pidDir)

	// Initialize TTS provider (optional)
	var ttsProvider tts.Provider
	if *ttsURL != "" {
		ttsProvider = tts.NewKokoroProvider(*ttsURL)
		log.Printf("TTS enabled: Kokoro at %s", *ttsURL)
	} else {
		log.Println("TTS disabled (no --tts-url provided)")
	}

	// Initialize STT provider (optional)
	var sttProvider stt.Provider
	if *sttURL != "" {
		sttProvider = stt.NewWhisperProvider(*sttURL)
		log.Printf("STT enabled: faster-whisper at %s", *sttURL)
	} else {
		log.Println("STT disabled (no --stt-url provided)")
	}

	// Initialize workspace scanner (optional)
	var scanner *workspace.Scanner
	if *workspaceDir != "" {
		scanner = workspace.NewScanner(*workspaceDir)
		if _, err := scanner.Scan(); err != nil {
			log.Fatalf("Failed to scan workspace: %v", err)
		}
		log.Printf("Workspace enabled: %s", *workspaceDir)
	} else {
		log.Println("Workspace disabled (no --workspace-dir provided)")
	}

	// Session map is always created for title persistence (and worktree mapping when workspace is enabled)
	sesMap, err := sessionmap.New(fmt.Sprintf("%s/session-map.json", *dataDir))
	if err != nil {
		log.Fatalf("Failed to load session map: %v", err)
	}
	log.Printf("Session map: %s/session-map.json", *dataDir)

	// Create API server
	server := api.NewServer(registry, ttsProvider, sttProvider, scanner, sesMap)

	// CORS middleware (for development; in production nginx handles this)
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
