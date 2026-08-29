package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"event-hunter/backend/internal/contexts/ingestion/ports"
)

func Handler(source ports.Source, repository ports.FailureRepository) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(writer http.ResponseWriter, _ *http.Request) {
		write(writer, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /health/ready", func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()
		if source.Ping(ctx) != nil || repository.Ping(ctx) != nil {
			write(writer, http.StatusServiceUnavailable, "not_ready")
			return
		}
		write(writer, http.StatusOK, "ready")
	})
	return mux
}

func write(writer http.ResponseWriter, status int, value string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"status": value})
}
