package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

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
		port         = flag.Int("port", 8080, "HTTP server port")
		hostname     = flag.String("hostname", "0.0.0.0", "Hostname to bind to")
		opencodeURL  = flag.String("opencode-url", "http://localhost:4096", "OpenCode server URL")
		ttsURL       = flag.String("tts-url", "", "TTS server URL (optional)")
		sttURL       = flag.String("stt-url", "", "faster-whisper server URL (optional)")
		workspaceDir = flag.String("workspace-dir", "", "Workspace directory for project/worktree discovery (optional)")
		dataDir      = flag.String("data-dir", "voilot-data", "Directory for persistent data (session map, etc.)")
		allowOrigins = flag.String("cors-origins", "*", "Allowed CORS origins (comma-separated)")
	)
	flag.Parse()

	// Initialize agent adapter
	agentAdapter := agent.NewOpenCodeAdapter(*opencodeURL)

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

	// Initialize workspace scanner and session map (optional)
	var scanner *workspace.Scanner
	var sesMap *sessionmap.Map
	if *workspaceDir != "" {
		scanner = workspace.NewScanner(*workspaceDir)
		if _, err := scanner.Scan(); err != nil {
			log.Fatalf("Failed to scan workspace: %v", err)
		}
		log.Printf("Workspace enabled: %s", *workspaceDir)

		var err error
		sesMap, err = sessionmap.New(fmt.Sprintf("%s/session-map.json", *dataDir))
		if err != nil {
			log.Fatalf("Failed to load session map: %v", err)
		}
		log.Printf("Session map: %s/session-map.json", *dataDir)
	} else {
		log.Println("Workspace disabled (no --workspace-dir provided)")
	}

	// Create API server
	server := api.NewServer(agentAdapter, ttsProvider, sttProvider, scanner, sesMap)

	// CORS middleware (for development; in production nginx handles this)
	handler := cors.New(cors.Options{
		AllowedOrigins:   []string{*allowOrigins},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	}).Handler(server)

	addr := fmt.Sprintf("%s:%d", *hostname, *port)
	log.Printf("voilot backend starting on %s", addr)
	log.Printf("OpenCode: %s", *opencodeURL)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
