package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"
)

var (
	interactedPodURI = os.Getenv("INTERACTED_POD_URI")
	pathPrefix       = os.Getenv("PATH_PREFIX")
	port             = os.Getenv("PORT")
	serviceName      = os.Getenv("SERVICE_NAME")

	httpClient = &http.Client{Timeout: 5 * time.Second}

	logger *slog.Logger
)

type (
	serviceResponse struct {
		ServiceName string `json:"serviceName"`
	}

	interactResponse struct {
		Message  string          `json:"message"`
		From     string          `json:"from"`
		Response serviceResponse `json:"response"`
	}
)

func errorHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func serviceHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, serviceResponse{ServiceName: serviceName})
}

func interactHandler(w http.ResponseWriter, r *http.Request) {
	target := interactedPodURI + "/service"
	logEvent(slog.LevelInfo, "interact.request.out", map[string]any{"target": target})

	resp, err := httpClient.Get(target)
	if err != nil {
		logEvent(slog.LevelError, "interact.response.error", map[string]any{"target": target, "error": err.Error()})
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	var svc serviceResponse
	if err := json.NewDecoder(resp.Body).Decode(&svc); err != nil {
		logEvent(slog.LevelError, "interact.response.error", map[string]any{"target": target, "error": err.Error()})
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	logEvent(slog.LevelInfo, "interact.response.in", map[string]any{
		"target":           target,
		"http.status_code": resp.StatusCode,
		"serviceName":      svc.ServiceName,
	})

	writeJSON(w, http.StatusOK, interactResponse{
		Message:  "success",
		From:     serviceName,
		Response: svc,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func logEvent(level slog.Level, event string, fields map[string]any) {
	attrs := make([]slog.Attr, 0, len(fields)+2)
	attrs = append(attrs, slog.String("event", event))
	attrs = append(attrs, slog.String("service", serviceName))

	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}

	logger.LogAttrs(context.Background(), level, event, attrs...)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		level := slog.LevelInfo
		if rec.status >= 500 {
			level = slog.LevelError
		}

		logEvent(level, "http.request.in", map[string]any{
			"method":           r.Method,
			"path":             r.URL.Path,
			"remoteAddr":       r.RemoteAddr,
			"http.status_code": rec.status,
			"durationMs":       time.Since(start).Milliseconds(),
		})
	})
}

func main() {
	if serviceName == "" || port == "" || interactedPodURI == "" || pathPrefix == "" {
		log.Fatal("SERVICE_NAME, PORT, INTERACTED_POD_URI must be set")
	}

	opts := &slog.HandlerOptions{
		AddSource: false,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				return slog.Attr{Key: "status", Value: slog.StringValue(a.Value.String())}
			}
			if a.Key == slog.TimeKey {
				return slog.Attr{Key: "timestamp", Value: a.Value}
			}
			return a
		},
	}

	logger = slog.New(slog.NewJSONHandler(os.Stdout, opts))
	slog.SetDefault(logger)

	mux := http.NewServeMux()
	mux.HandleFunc(fmt.Sprintf("/%s/error", pathPrefix), errorHandler)
	mux.HandleFunc(fmt.Sprintf("/%s/service", pathPrefix), serviceHandler)
	mux.HandleFunc(fmt.Sprintf("/%s/interact", pathPrefix), interactHandler)

	logger.Info(fmt.Sprintf("starting %s on :%s (interacting with %s)", serviceName, port, interactedPodURI))

	if err := http.ListenAndServe(":"+port, loggingMiddleware(mux)); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}
