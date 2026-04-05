package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/jenpet/voilot/internal/agent"
	"github.com/jenpet/voilot/internal/api"
	"github.com/jenpet/voilot/internal/stt"
	"github.com/jenpet/voilot/internal/tts"
	"github.com/rs/cors"
)

func main() {
	var (
		port         = flag.Int("port", 8080, "HTTP server port")
		hostname     = flag.String("hostname", "0.0.0.0", "Hostname to bind to")
		opencodeURL  = flag.String("opencode-url", "http://localhost:4096", "OpenCode server URL")
		ttsURL       = flag.String("tts-url", "", "TTS server URL (optional)")
		sttURL       = flag.String("stt-url", "", "faster-whisper server URL (optional)")
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

	// Create API server
	server := api.NewServer(agentAdapter, ttsProvider, sttProvider)

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
