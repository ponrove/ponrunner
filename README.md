# Ponrunner

[![Codacy Badge](https://app.codacy.com/project/badge/Grade/d4483525f5094ffdae58fd56d1d7a4ff)](https://app.codacy.com/gh/ponrove/ponrunner/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade)
[![Codacy Badge](https://app.codacy.com/project/badge/Coverage/d4483525f5094ffdae58fd56d1d7a4ff)](https://app.codacy.com/gh/ponrove/ponrunner/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_coverage)
[![GoDoc](https://godoc.org/github.com/ponrove/ponrunner?status.svg)](https://godoc.org/github.com/ponrove/ponrunner)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Description

`ponrunner` is a Go package designed to simplify the setup, execution, and graceful shutdown of HTTP servers. It seamlessly integrates with the [Chi router](https://github.com/go-chi/chi) and the [Huma v2 API framework](https://github.com/danielgtaylor/huma), promoting a configuration-driven approach using the [configura](https://github.com/ponrove/configura) package.

With `ponrunner`, you can quickly bootstrap a robust server, allowing you to focus more on your API logic and less on boilerplate server code.

**Key Features:**

- **Simplified Server Lifecycle:** Easy-to-use `Start` function to initialize and run your server.
- **Graceful Shutdown:** Handles OS signals (SIGHUP, SIGINT, SIGTERM, SIGQUIT) for a clean shutdown process, ensuring active requests are completed.
- **Configuration Driven:** Leverages `configura` for managing server parameters like port and timeouts through environment variables or other sources.
- **Chi & Huma Integration:** Built to work smoothly with Chi for routing and Huma for API definition and documentation.
- **Modular API Bundles:** Define your API endpoints in self-contained `APIBundle` functions, promoting a clean and organized codebase.
- **Middleware Ready:** Comes with sensible default middleware like request ID, request timeout, and structured request logging.
- **Configurable Timeouts:** Set server read, write, request, and shutdown timeouts via configuration.

## Dependencies

- [Chi](https://github.com/go-chi/chi/v5)
- [Huma v2](https://github.com/danielgtaylor/huma/v2)
- [Configura](https://github.com/ponrove/configura)
- [Zerolog](https://github.com/rs/zerolog)

## Usage

To use `ponrunner`, you'll typically define your API logic in one or more `APIBundle` functions, configure your server settings, and then call `ponrunner.Start`.

### 1. Define Configuration

`ponrunner` expects certain configuration variables to be available through a `configura.Config` instance. You should define these in your application:

- `SERVER_PORT`: The port on which the server will listen (e.g., `8080`).
- `SERVER_REQUEST_TIMEOUT`: Maximum duration for processing a request (e.g., `15` seconds).
  - _Note_: Due to the current definition of `SERVER_WRITE_TIMEOUT` in `ponrunner` (`configura.Variable[int64] = "SERVER_REQUEST_TIMEOUT"`), this value is also used for the HTTP server's `WriteTimeout`.
- `SERVER_READ_TIMEOUT`: Maximum duration for reading the entire request, including the body (e.g., `10` seconds).
- `SERVER_SHUTDOWN_TIMEOUT`: Maximum duration allowed for graceful shutdown (e.g., `30` seconds).

Here's an example of how you might set up `configura` in your `main.go`:

```go
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
```

Running the Example

1.  Save the `main.go` and `bundles.go` (if separate, or combine them).
2.  Ensure you have the necessary Go modules:
    ```sh
    go get github.com/ponrove/ponrunner
    go get github.com/ponrove/configura
    go get github.com/go-chi/chi/v5
    go get github.com/danielgtaylor/huma/v2
    go get github.com/rs/zerolog
    ```
3.  Set environment variables (optional, defaults will be used otherwise):
    ```sh
    export SERVER_PORT=8888
    export SERVER_REQUEST_TIMEOUT=20
    ```
4.  Run the application:
    ```sh
    go run main.go
    ```
5.  Access the endpoints:
    - `http://localhost:8888/hello`
    - `http://localhost:8888/openapi.json` (Huma serves OpenAPI spec by default)

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
