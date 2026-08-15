package health

import (
	"encoding/json"
	"net/http"
)

type Response struct {
	Healthy bool   `json:"healthy"`
	Status  string `json:"status"`
	Service string `json:"service"`
	Port    int    `json:"port,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Handler returns a /health HTTP handler reporting the given service's
// startup config state. A non-nil configErr marks the service unhealthy.
func Handler(service string, port int, configErr error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if configErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(Response{
				Healthy: false,
				Status:  "unhealthy",
				Service: service,
				Error:   configErr.Error(),
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Response{
			Healthy: true,
			Status:  "Ok",
			Service: service,
			Port:    port,
		})
	}
}
