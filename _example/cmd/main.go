package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/ponrove/configura"
	"github.com/ponrove/ponrunner"
)

func main() {
	ctx := context.Background()
	// Initialize Configuration with configura
	cfg := configura.NewConfigImpl()
	configura.LoadEnvironment(cfg, ponrunner.SERVER_PORT, 8080)                                // Fallback port 8080
	configura.LoadEnvironment(cfg, ponrunner.SERVER_WRITE_TIMEOUT, int64(5))                   // Fallback write timeout 5 seconds
	configura.LoadEnvironment(cfg, ponrunner.SERVER_READ_TIMEOUT, int64(5))                    // Fallback read timeout 5 seconds
	configura.LoadEnvironment(cfg, ponrunner.SERVER_REQUEST_TIMEOUT, int64(5))                 // Fallback request timeout 5 seconds
	configura.LoadEnvironment(cfg, ponrunner.SERVER_SHUTDOWN_TIMEOUT, int64(5))                // Fallback shutdown timeout 5 seconds
	configura.LoadEnvironment(cfg, ponrunner.SERVER_OPENFEATURE_PROVIDER_NAME, "NoopProvider") // Fallback to NoopProvider
	configura.LoadEnvironment(cfg, ponrunner.SERVER_OPENFEATURE_PROVIDER_URL, "")              // No URL for NoopProvider
	configura.LoadEnvironment(cfg, ponrunner.OTEL_ENABLED, false)                              // Disable OpenTelemetry by default
	configura.LoadEnvironment(cfg, ponrunner.OTEL_LOGS_ENABLED, true)                          // Enable OpenTelemetry logs by default
	configura.LoadEnvironment(cfg, ponrunner.OTEL_METRICS_ENABLED, true)
	configura.LoadEnvironment(cfg, ponrunner.OTEL_TRACES_ENABLED, true)
	configura.LoadEnvironment(cfg, ponrunner.OTEL_SERVICE_NAME, "example-service")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_ENDPOINT, "")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_PROTOCOL, "")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_HEADERS, "")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_TIMEOUT, 10)
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_LOGS_ENDPOINT, "")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_LOGS_PROTOCOL, "")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_LOGS_HEADERS, "")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_LOGS_TIMEOUT, 10)
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_METRICS_ENDPOINT, "")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_METRICS_PROTOCOL, "")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_METRICS_HEADERS, "")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_METRICS_TIMEOUT, 10)
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_TRACES_ENDPOINT, "")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_TRACES_PROTOCOL, "")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_TRACES_HEADERS, "")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_TRACES_TIMEOUT, 10)
	configura.LoadEnvironment(cfg, ponrunner.SERVER_LOG_LEVEL, "info")
	configura.LoadEnvironment(cfg, ponrunner.SERVER_LOG_FORMAT, "json")
	configura.LoadEnvironment(cfg, ponrunner.OTEL_EXPORTER_OTLP_ENDPOINT, "")

	r := chi.NewRouter()
	err := ponrunner.Start(ctx, cfg, r, func(c configura.Config, r chi.Router, a huma.API) error {
		return ponrunner.RegisterAPIBundles(c, a, MyAPIBundle)
	})
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

type (
	MyAPIBundleRequest  struct{}
	MyAPIBundleResponse struct {
		Status int `header:"-"`
		Body   struct {
			Message string `json:"message"`
		}
	}
)

// MyAPIBundle is an example APIBundle.
func MyAPIBundle(cfg configura.Config, api huma.API) error {
	huma.Register(api, huma.Operation{
		OperationID: "get-hello",
		Method:      http.MethodGet,
		Path:        "/hello",
		Summary:     "Says hello",
		Description: "A simple endpoint that returns a greeting.",
	}, func(ctx context.Context, input *MyAPIBundleRequest) (*MyAPIBundleResponse, error) {
		return &MyAPIBundleResponse{
			Status: http.StatusOK,
			Body: struct {
				Message string `json:"message"`
			}{
				Message: fmt.Sprintf("Hello from Ponrunner! Port: %d", cfg.Int64("SERVER_PORT")),
			},
		}, nil
	})
	return nil
}
