package ponrunner

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	chim "github.com/go-chi/chi/v5/middleware"
	"github.com/ponrove/configura"
	"github.com/ponrove/ponrunner/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	SERVER_PORT             configura.Variable[int64]  = "SERVER_PORT"
	SERVER_WRITE_TIMEOUT    configura.Variable[int64]  = "SERVER_REQUEST_TIMEOUT"
	SERVER_READ_TIMEOUT     configura.Variable[int64]  = "SERVER_READ_TIMEOUT"
	SERVER_REQUEST_TIMEOUT  configura.Variable[int64]  = "SERVER_REQUEST_TIMEOUT"
	SERVER_SHUTDOWN_TIMEOUT configura.Variable[int64]  = "SERVER_SHUTDOWN_TIMEOUT"
	SERVER_LOG_LEVEL        configura.Variable[string] = "SERVER_LOG_LEVEL"
	SERVER_LOG_FORMAT       configura.Variable[string] = "SERVER_LOG_FORMAT"
)

// APIBundle is a function type that takes a configura.Config and huma.API,
type APIBundle func(configura.Config, huma.API) error

// RegisterAPIBundles registers multiple API bundles with the provided configuration and API instance.
func RegisterAPIBundles(cfg configura.Config, api huma.API, bundles ...APIBundle) error {
	for _, bundle := range bundles {
		if err := bundle(cfg, api); err != nil {
			return err
		}
	}

	return nil
}

// serverControl defines an interface for server operations needed for shutdown.
// This allows for easier testing by mocking an http.Server.
type serverControl interface {
	Shutdown(ctx context.Context) error
}

// handleServerShutdown performs a graceful shutdown of an HTTP server-like component.
// It takes a parent context (for the shutdown operation itself),
// the server component to shut down, and a timeout duration for the shutdown process.
func handleServerShutdown(shutdownParentCtx context.Context, srv serverControl, shutdownTimeout time.Duration) error {
	slog.Info("Initiating server shutdown sequence...")

	// Create a context with a timeout for the srv.Shutdown call.
	shutdownOpCtx, cancelShutdownOp := context.WithTimeout(shutdownParentCtx, shutdownTimeout)
	defer cancelShutdownOp()

	// Goroutine to log if the shutdown operation context itself times out or is canceled.
	go func() {
		<-shutdownOpCtx.Done()
		if shutdownOpCtx.Err() == context.DeadlineExceeded {
			slog.Warn("Shutdown operation context deadline exceeded. Server might be taking too long to close active connections.")
		} else if shutdownOpCtx.Err() == context.Canceled { // This case handles when shutdownParentCtx is canceled.
			slog.Info("Shutdown operation context was canceled by its parent.")
		}
	}()

	// Attempt to gracefully shut down the server.
	// srv.Shutdown is expected to respect the shutdownOpCtx.
	err := srv.Shutdown(shutdownOpCtx)
	if err != nil {
		if err == http.ErrServerClosed {
			slog.Info("Server was already closed.")
			return nil // Not an error in this context; shutdown is effectively complete.
		}
		slog.Error("Error during server shutdown", slog.Any("error", err))
		return err
	}

	slog.Info("Server shutdown gracefully completed.")
	return nil
}

type RegisterRoutes func(configura.Config, chi.Router, huma.API) error

// Start initializes and starts the Ponrove server. It sets up the HTTP server with the provided configuration and API
// bundles, and handles graceful shutdown on receiving OS signals.
func Start(ctx context.Context, cfg configura.Config, router chi.Router, register RegisterRoutes) error {
	// Ensure the configuration contains all required keys, it's up to the caller to ensure that the configuration
	// is loaded with the necessary values before calling Start.
	err := cfg.ConfigurationKeysRegistered(
		SERVER_PORT,
		SERVER_REQUEST_TIMEOUT,
		SERVER_READ_TIMEOUT,
		SERVER_WRITE_TIMEOUT,
		SERVER_SHUTDOWN_TIMEOUT,
		SERVER_OPENFEATURE_PROVIDER_NAME,
		SERVER_OPENFEATURE_PROVIDER_URL,
		SERVER_LOG_LEVEL,
		SERVER_LOG_FORMAT,
	)
	if err != nil {
		return err
	}

	// Set up the logger based on the configuration.
	logLevelStr := configura.Fallback(cfg.String(SERVER_LOG_LEVEL), "info")
	var logLevel slog.Level
	switch logLevelStr {
	case "trace": // slog doesn't have trace, map to debug
		logLevel = slog.LevelDebug
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		slog.WarnContext(ctx, "Unsupported or unmappable log level configured, defaulting to INFO", slog.String("configuredLevel", logLevelStr))
		logLevel = slog.LevelInfo // Default to Info level.
	}

	logFormat := configura.Fallback(cfg.String(SERVER_LOG_FORMAT), "text") // Default to text
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: logLevel}

	if logFormat == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	defaultLogger := slog.New(handler)
	slog.SetDefault(defaultLogger)

	// Set the open feature provider if configured.
	err = setOpenFeatureProvider(cfg)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to set OpenFeature provider", slog.Any("error", err))
		return err // Return early if OpenFeature provider setup fails.
	}

	// Initialize OpenTelemetry if enabled
	// Pass the slog-augmented context to setupOTelSDK
	otelShutdown, otelSetupErr := setupOTelSDK(ctx, cfg)
	if otelSetupErr != nil {
		return fmt.Errorf("Failed to setup OpenTelemetry SDK: %w", otelSetupErr)
	} else if otelShutdown != nil { // Check otelShutdown is not nil (i.e., OTel actually initialized)
		slog.InfoContext(ctx, "OpenTelemetry SDK initialized successfully.")
		// Defer the shutdown of OpenTelemetry to ensure it's called on exit.
		// Use a background context for shutdown as the main context might be canceled.
		defer func() {
			// Use a background context for OTel shutdown, as the parent ctx might be done.
			// Logs from here will use the slog.Default logger.
			if err := otelShutdown(context.Background()); err != nil {
				slog.ErrorContext(ctx, "error during OpenTelemetry shutdown", slog.Any("error", err))
			}
		}()
	}

	// serverCtx is canceled when an OS signal is received, used for server's BaseContext.
	serverCtx, stopSignalNotify := signal.NotifyContext(ctx, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stopSignalNotify() // Ensures signal notifications are stopped when Runtime exits.

	router.Use(
		middleware.IPAddress(cfg), // Adds the client's IP address to the request context.
		chim.RequestID,            // Adds a unique request ID to each request.
		chim.Recoverer,
		chim.Timeout(time.Duration(cfg.Int64(SERVER_REQUEST_TIMEOUT))*time.Second),
		middleware.LogRequest(cfg), // Custom middleware to log requests.
	)

	h := humachi.New(router, huma.DefaultConfig("Ponrove Backend API", "1.0.0"))

	if err := register(cfg, router, h); err != nil {
		slog.ErrorContext(ctx, "Failed to register routes", slog.Any("error", err))
		return err
	}

	srv := &http.Server{ // Use a pointer to satisfy serverControl if http.Server is passed directly.
		Addr: fmt.Sprintf(":%d", cfg.Int64(SERVER_PORT)),
		// BaseContext ensures the server stops accepting new connections when serverCtx is canceled.
		BaseContext:  func(_ net.Listener) context.Context { return serverCtx },
		ReadTimeout:  time.Duration(cfg.Int64(SERVER_READ_TIMEOUT)) * time.Second,
		WriteTimeout: time.Duration(cfg.Int64(SERVER_WRITE_TIMEOUT)) * time.Second,
		Handler:      router, // This will be wrapped if OTel is enabled
	}

	// Wrap the main router with OpenTelemetry HTTP instrumentation if enabled
	if otelShutdown != nil { // otelShutdown check ensures setup was successful
		slog.InfoContext(ctx, "Wrapping HTTP handler with OpenTelemetry instrumentation.")
		srv.Handler = otelhttp.NewHandler(router, "http.server")
	}

	srvListenAndServeErrChan := make(chan error, 1)
	go func() {
		slog.InfoContext(ctx, "Starting server", slog.String("address", srv.Addr))
		// ListenAndServe blocks until the server is shut down.
		// It returns http.ErrServerClosed if Shutdown is called successfully.
		lsErr := srv.ListenAndServe()
		if lsErr != nil && lsErr != http.ErrServerClosed {
			srvListenAndServeErrChan <- lsErr
		} else {
			// If http.ErrServerClosed or nil, server stopped as expected.
			slog.InfoContext(ctx, "ListenAndServe returned", slog.Any("error", lsErr)) // slog.Any handles nil error gracefully
			close(srvListenAndServeErrChan)                                            // Signal clean exit or expected closure by closing the channel.
		}
	}()

	var listenAndServeError error
	select {
	case err, ok := <-srvListenAndServeErrChan:
		if ok { // An actual error was sent from ListenAndServe (channel not closed).
			listenAndServeError = err
			slog.ErrorContext(ctx, "Server stopped due to an error from ListenAndServe.", slog.Any("error", listenAndServeError))
		} else { // Channel closed: ListenAndServe returned nil or http.ErrServerClosed.
			slog.InfoContext(ctx, "Server stopped (ListenAndServe returned nil or http.ErrServerClosed before any signal).")
		}
	case <-serverCtx.Done(): // OS signal received. serverCtx is now canceled.
		slog.InfoContext(ctx, "Shutdown signal received. serverCtx is Done. Proceeding to shutdown.")
		// stopSignalNotify() is deferred. It will clean up signal handling.
		// If we call stopSignalNotify() here, it ensures no new signals for this NotifyContext are processed during shutdown.
		// This can be useful if shutdown is lengthy. signal.Stop is safe to call multiple times.
		stopSignalNotify()
	}

	// Proceed with shutdown logic regardless of how the select statement was exited.
	slog.InfoContext(ctx, "Initiating shutdown procedure via handleServerShutdown...")
	shutdownTimeout := time.Duration(cfg.Int64(SERVER_SHUTDOWN_TIMEOUT)) * time.Second
	shutdownErr := handleServerShutdown(context.Background(), srv, shutdownTimeout)

	if listenAndServeError != nil {
		// If ListenAndServe failed, that's the primary error to return.
		if shutdownErr != nil {
			slog.ErrorContext(ctx, "Additional error during shutdown attempt after ListenAndServe failure.", slog.Any("error", shutdownErr))
		}
		return listenAndServeError
	}

	// If ListenAndServe did not error out (i.e., it was http.ErrServerClosed or a signal was received),
	// then the result of the shutdown attempt is the final outcome.
	return shutdownErr
}
