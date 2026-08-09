package main

import (
	"embodied-ai-proxy/backend/internal/config"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Port    string `json:"port"`
}

func healthHandler(config *config.AppConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := HealthResponse{
			Status:  "Ok",
			Service: "embodied-ai-proxy-backend",
			Port:    config.Port,
		}
		json.NewEncoder(w).Encode(response)
	}
}

func main() {
	appConfig, err := config.LoadConfig("configs/config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler(appConfig))

	address := fmt.Sprintf(":%s", appConfig.Port)
	log.Printf("Starting Embodied AI Proxy Go Backend on http://localhost%s...", address)

	if err := http.ListenAndServe(address, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
