package monitoring

import (
	"embodied-ai-proxy/backend/internal/config"
	"encoding/json"
	"net/http"
)

type HealthResponse struct {
	Healthy bool   `json:"healthy"`
	Status  string `json:"status"`
	Service string `json:"service"`
	Port    int    `json:"port,omitempty"`
	Error   string `json:"error,omitempty"`
}

func HealthHandler(config *config.AppConfig, configErr error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if configErr != nil || config == nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(HealthResponse{
				Healthy: false,
				Status:  "unhealthy",
				Service: "embodied-ai-proxy-backend",
				Error:   configErr.Error(),
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(HealthResponse{
			Healthy: true,
			Status:  "Ok",
			Service: "embodied-ai-proxy-backend",
			Port:    config.Port,
		})
	}
}
