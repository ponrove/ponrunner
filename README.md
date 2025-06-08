# Ponrunner

[![Codacy Badge](https://app.codacy.com/project/badge/Grade/d4483525f5094ffdae58fd56d1d7a4ff)](https://app.codacy.com/gh/ponrove/ponrunner/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade)
[![Codacy Badge](https://app.codacy.com/project/badge/Coverage/d4483525f5094ffdae58fd56d1d7a4ff)](https://app.codacy.com/gh/ponrove/ponrunner/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_coverage)
[![GoDoc](https://godoc.org/github.com/ponrove/ponrunner?status.svg)](https://godoc.org/github.com/ponrove/ponrunner)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Description

`ponrunner` is a Go package designed to simplify the setup, execution, and graceful shutdown of HTTP servers. It seamlessly integrates with the [Chi router](https://github.com/go-chi/chi) and the [Huma v2 API framework](https://github.com/danielgtaylor/huma), promoting a configuration-driven approach using the [configura](https://github.com/ponrove/configura) package.

With `ponrunner`, you can quickly bootstrap a robust, observable, and feature-flag-aware server, allowing you to focus more on your API logic and less on boilerplate code.

**Key Features:**

- **Simplified Server Lifecycle:** Easy-to-use `Start` function to initialize and run your server.
- **Graceful Shutdown:** Handles OS signals for a clean shutdown process.
- **Configuration Driven:** Leverages `configura` for managing all server parameters via environment variables.
- **Observability Ready:** Built-in, configurable OpenTelemetry support for traces, metrics, and logs.
- **Feature Flagging:** Integrated with [OpenFeature](https://openfeature.dev/) for dynamic feature management.
- **Structured Logging:** Uses Go's standard `log/slog` for structured, leveled logging (JSON or text).
- **Chi & Huma Integration:** Built to work smoothly with Chi for routing and Huma for API definition.
- **Middleware Ready:** Comes with default middleware for request ID, request logging, IP address handling, and timeouts.

## Dependencies

- [Go 1.21+](https://go.dev/dl/) (for `slog`)
- [Chi](https://github.com/go-chi/chi/v5)
- [Huma v2](https://github.com/danielgtaylor/huma/v2)
- [Configura](https://github.com/ponrove/configura)
- [OpenTelemetry](https://opentelemetry.io/)
- [OpenFeature](https://openfeature.dev/)

## Example: Full Observability Stack

For a comprehensive example of how to run `ponrunner` with a full observability stack (including Grafana, Loki, Prometheus, Jaeger, and Tempo), please see the README in the example directory:

➡️ **[Full Docker Compose Example](./_example/README.md)**

## Usage

### 1. Configuration Variables

`ponrunner` is configured entirely through environment variables, which are loaded via the `configura` package.

#### Server & Logging

- `SERVER_PORT`: The port for the server to listen on (e.g., `8080`).
- `SERVER_REQUEST_TIMEOUT`: Max duration for a request (e.g., `15`).
- `SERVER_READ_TIMEOUT`: Max duration for reading a request body (e.g., `10`).
- `SERVER_WRITE_TIMEOUT`: Max duration for writing a response (e.g., `10`).
- `SERVER_SHUTDOWN_TIMEOUT`: Max duration for graceful shutdown (e.g., `30`).
- `SERVER_LOG_LEVEL`: Log level (`debug`, `info`, `warn`, `error`).
- `SERVER_LOG_FORMAT`: Log format (`text` or `json`).

#### OpenFeature

- `SERVER_OPENFEATURE_PROVIDER_NAME`: Name of the provider (e.g., `go-feature-flag`). Defaults to `NoopProvider`.
- `SERVER_OPENFEATURE_PROVIDER_URL`: URL of the provider endpoint.

#### OpenTelemetry

- `OTEL_ENABLED`: Set to `true` to enable OpenTelemetry instrumentation.
- `OTEL_SERVICE_NAME`: The name of your service (e.g., `my-cool-api`).
- `OTEL_TRACES_ENABLED`, `OTEL_METRICS_ENABLED`, `OTEL_LOGS_ENABLED`: Set to `true` or `false` to toggle individual signals.
- `OTEL_EXPORTER_OTLP_ENDPOINT`: Default OTLP endpoint URL (e.g., `http://opentelemetry-collector:4317`).
- `OTEL_EXPORTER_OTLP_PROTOCOL`: Default protocol for all signals (`grpc` or `http/protobuf`).
- `OTEL_EXPORTER_OTLP_HEADERS`: Default headers for all signals (e.g., `key=value,key2=value2`).
- `OTEL_EXPORTER_OTLP_TIMEOUT`: Default export timeout for all signals.

You can also override settings for each signal type (traces, metrics, logs) using specific variables like `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, `OTEL_EXPORTER_OTLP_METRICS_PROTOCOL`, etc.

### 2. Example: Manual Setup

Here's how to set up and run a `ponrunner` server manually.

```go
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

	// 1. Initialize Configuration
	// Ponrunner requires several configuration keys. Your application is responsible
	// for registering them with 'configura'.
	cfg := configura.NewConfigImpl()
	registerConfig(cfg)

	// 2. Create a Chi router
	router := chi.NewRouter()

	// 3. Start the server
	// The third argument is a function that registers your application's routes.
	err := ponrunner.Start(ctx, cfg, router, registerRoutes)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// registerConfig registers all the necessary configuration variables that ponrunner uses.
func registerConfig(cfg configura.Config) {
	// In a real app, you would load these from the environment.
	// We set fallback values here for demonstration.
	configura.LoadEnvironment(cfg, ponrunner.SERVER_PORT, int64(8080))
	configura.LoadEnvironment(cfg, ponrunner.SERVER_WRITE_TIMEOUT, int64(10))
	configura.LoadEnvironment(cfg, ponrunner.SERVER_READ_TIMEOUT, int64(10))
	configura.LoadEnvironment(cfg, ponrunner.SERVER_REQUEST_TIMEOUT, int64(15))
	configura.LoadEnvironment(cfg, ponrunner.SERVER_SHUTDOWN_TIMEOUT, int64(30))
	configura.LoadEnvironment(cfg, ponrunner.SERVER_LOG_LEVEL, "info")
	configura.LoadEnvironment(cfg, ponrunner.SERVER_LOG_FORMAT, "text")
	configura.LoadEnvironment(cfg, ponrunner.SERVER_OPENFEATURE_PROVIDER_NAME, "")
	configura.LoadEnvironment(cfg, ponrunner.SERVER_OPENFEATURE_PROVIDER_URL, "")
	// OTel variables are also registered but default to disabled.
}

type HelloResponse struct {
	Body struct {
		Message string `json:"message"`
	}
}

// registerRoutes defines your API endpoints.
// It receives the config, the Chi router, and a Huma API instance.
func registerRoutes(cfg configura.Config, router chi.Router, api huma.API) error {
	// Register Huma-powered routes
	huma.Register(api, huma.Operation{
		OperationID: "get-hello-huma",
		Method:      http.MethodGet,
		Path:        "/hello",
		Summary:     "Get a greeting from Huma",
	}, func(ctx context.Context, input *struct{}) (*HelloResponse, error) {
		resp := &HelloResponse{}
		port := cfg.Int64(ponrunner.SERVER_PORT)
		resp.Body.Message = fmt.Sprintf("Hello from Ponrunner with Huma on port %d!", port)
		return resp, nil
	})

	// You can also register standard Chi routes
	router.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})

	return nil
}
```

### 3. Running the Example

1.  Save the code above as `main.go`.
2.  Initialize your Go module and fetch dependencies:
    ```sh
    go mod init myapp
    go get github.com/ponrove/ponrunner
    ```
3.  Run the application:
    ```sh
    go run main.go
    ```
4.  Set environment variables to customize its behavior:
    ```sh
    export SERVER_PORT=8888
    export SERVER_LOG_FORMAT=json
    go run main.go
    ```
5.  Access the endpoints:
    - `curl http://localhost:8888/hello`
    - `curl http://localhost:8888/ping`
    - Huma also serves an OpenAPI spec: `http://localhost:8888/openapi.json`

## Contributing

Contributions are welcome! Please feel free to open a pull request with any improvements, bug fixes, or new features.

1.  Fork the repository.
2.  Create your feature branch (`git checkout -b feature/AmazingFeature`).
3.  Commit your changes (`git commit -m 'Add some AmazingFeature'`).
4.  Push to the branch (`git push origin feature/AmazingFeature`).
5.  Open a Pull Request.

Please ensure your code is well-tested and follows Go best practices.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
