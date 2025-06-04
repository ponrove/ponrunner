package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/ponrove/configura"
	"github.com/ponrove/ponrunner"
)

func main() {
	ctx := context.Background()

	// --- Simulate setting environment variables (for example purposes) ---
	// In a real scenario, these would be set in your shell, Dockerfile, K8s manifest, etc.
	os.Setenv(string(ponrunner.SERVER_PORT), "8080")
	os.Setenv(string(ponrunner.SERVER_WRITE_TIMEOUT), "5")
	os.Setenv(string(ponrunner.SERVER_READ_TIMEOUT), "5")
	os.Setenv(string(ponrunner.SERVER_REQUEST_TIMEOUT), "5")
	os.Setenv(string(ponrunner.SERVER_SHUTDOWN_TIMEOUT), "5")

	// Initialize Configuration with configura
	cfg := configura.NewConfigImpl()
	configura.LoadEnvironment(cfg, ponrunner.SERVER_PORT, 8080)                 // Fallback port 8080
	configura.LoadEnvironment(cfg, ponrunner.SERVER_WRITE_TIMEOUT, int64(5))    // Fallback write timeout 5 seconds
	configura.LoadEnvironment(cfg, ponrunner.SERVER_READ_TIMEOUT, int64(5))     // Fallback read timeout 5 seconds
	configura.LoadEnvironment(cfg, ponrunner.SERVER_REQUEST_TIMEOUT, int64(5))  // Fallback request timeout 5 seconds
	configura.LoadEnvironment(cfg, ponrunner.SERVER_SHUTDOWN_TIMEOUT, int64(5)) // Fallback shutdown timeout 5 seconds

	r := chi.NewRouter()
	err := ponrunner.Start(ctx, cfg, r, MyAPIBundle)
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
