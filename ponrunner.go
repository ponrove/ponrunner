package ponrunner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	chim "github.com/go-chi/chi/v5/middleware"
	"github.com/ponrove/configura"
	"github.com/ponrove/ponrunner/middleware"
	"github.com/rs/zerolog/log"
)

const (
	SERVER_PORT             configura.Variable[int64] = "SERVER_PORT"
	SERVER_WRITE_TIMEOUT    configura.Variable[int64] = "SERVER_REQUEST_TIMEOUT"
	SERVER_READ_TIMEOUT     configura.Variable[int64] = "SERVER_READ_TIMEOUT"
	SERVER_REQUEST_TIMEOUT  configura.Variable[int64] = "SERVER_REQUEST_TIMEOUT"
	SERVER_SHUTDOWN_TIMEOUT configura.Variable[int64] = "SERVER_SHUTDOWN_TIMEOUT"
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

// handleShutdown performs a graceful shutdown of an HTTP server-like component.
// It takes a parent context (for the shutdown operation itself),
// the server component to shut down, and a timeout duration for the shutdown process.
func handleShutdown(shutdownParentCtx context.Context, srv serverControl, shutdownTimeout time.Duration) error {
	log.Info().Msg("Initiating server shutdown sequence...")

	// Create a context with a timeout for the srv.Shutdown call.
	shutdownOpCtx, cancelShutdownOp := context.WithTimeout(shutdownParentCtx, shutdownTimeout)
	defer cancelShutdownOp()

	// Goroutine to log if the shutdown operation context itself times out or is canceled.
	go func() {
		<-shutdownOpCtx.Done()
		if shutdownOpCtx.Err() == context.DeadlineExceeded {
			log.Warn().Msg("Shutdown operation context deadline exceeded. Server might be taking too long to close active connections.")
		} else if shutdownOpCtx.Err() == context.Canceled { // This case handles when shutdownParentCtx is canceled.
			log.Info().Msg("Shutdown operation context was canceled by its parent.")
		}
	}()

	// Attempt to gracefully shut down the server.
	// srv.Shutdown is expected to respect the shutdownOpCtx.
	err := srv.Shutdown(shutdownOpCtx)
	if err != nil {
		if err == http.ErrServerClosed {
			log.Info().Msg("Server was already closed.")
			return nil // Not an error in this context; shutdown is effectively complete.
		}
		log.Error().Err(err).Msg("Error during server shutdown")
		return err
	}

	log.Info().Msg("Server shutdown gracefully completed.")
	return nil
}

// Start initializes and starts the Ponrove server. It sets up the HTTP server with the provided configuration and API
// bundles, and handles graceful shutdown on receiving OS signals.
func Start(ctx context.Context, cfg configura.Config, router chi.Router, bundles ...APIBundle) error {
	err := cfg.ConfigurationKeysRegistered(
		SERVER_PORT,
		SERVER_REQUEST_TIMEOUT,
		SERVER_READ_TIMEOUT,
		SERVER_WRITE_TIMEOUT,
		SERVER_SHUTDOWN_TIMEOUT,
	)
	if err != nil {
		return err
	}

	// serverCtx is canceled when an OS signal is received, used for server's BaseContext.
	serverCtx, stopSignalNotify := signal.NotifyContext(ctx, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stopSignalNotify() // Ensures signal notifications are stopped when Runtime exits.

	router.Use(
		chim.RequestID, // Adds a unique request ID to each request.
		chim.Recoverer,
		chim.Timeout(time.Duration(cfg.Int64(SERVER_REQUEST_TIMEOUT))*time.Second),
		middleware.LogRequest(cfg), // Custom middleware to log requests.

	)

	h := humachi.New(router, huma.DefaultConfig("Ponrove Backend API", "1.0.0"))

	if err := RegisterAPIBundles(cfg, h, bundles...); err != nil {
		log.Error().Err(err).Msg("failed to register API bundles")
		return err // Return early if API bundle registration fails.
	}

	srv := &http.Server{ // Use a pointer to satisfy serverControl if http.Server is passed directly.
		Addr: fmt.Sprintf(":%d", cfg.Int64(SERVER_PORT)),
		// BaseContext ensures the server stops accepting new connections when serverCtx is canceled.
		BaseContext:  func(_ net.Listener) context.Context { return serverCtx },
		ReadTimeout:  time.Duration(cfg.Int64(SERVER_READ_TIMEOUT)) * time.Second,
		WriteTimeout: time.Duration(cfg.Int64(SERVER_WRITE_TIMEOUT)) * time.Second,
		Handler:      router,
	}

	srvListenAndServeErrChan := make(chan error, 1)
	go func() {
		log.Info().Msgf("starting server on %s", srv.Addr)
		// ListenAndServe blocks until the server is shut down.
		// It returns http.ErrServerClosed if Shutdown is called successfully.
		lsErr := srv.ListenAndServe()
		if lsErr != nil && lsErr != http.ErrServerClosed {
			srvListenAndServeErrChan <- lsErr
		} else {
			// If http.ErrServerClosed or nil, server stopped as expected.
			log.Info().Msgf("listenAndServe returned: %v", lsErr)
			close(srvListenAndServeErrChan) // Signal clean exit or expected closure by closing the channel.
		}
	}()

	var listenAndServeError error
	// Wait for either a shutdown signal (via serverCtx.Done()) or the server to stop on its own (error or graceful).
	select {
	case err, ok := <-srvListenAndServeErrChan:
		if ok { // An actual error was sent from ListenAndServe (channel not closed).
			listenAndServeError = err
			log.Error().Err(listenAndServeError).Msg("Server stopped due to an error from ListenAndServe.")
		} else { // Channel closed: ListenAndServe returned nil or http.ErrServerClosed.
			log.Info().Msg("server stopped (ListenAndServe returned nil or http.ErrServerClosed before any signal).")
		}
	case <-serverCtx.Done(): // OS signal received. serverCtx is now canceled.
		log.Info().Msg("shutdown signal received. serverCtx is Done. Proceeding to shutdown.")
		// stopSignalNotify() is deferred. It will clean up signal handling.
		// If we call stopSignalNotify() here, it ensures no new signals for this NotifyContext are processed during shutdown.
		// This can be useful if shutdown is lengthy. signal.Stop is safe to call multiple times.
		stopSignalNotify()
	}

	// Proceed with shutdown logic regardless of how the select statement was exited.
	log.Info().Msg("initiating shutdown procedure via handleShutdown...")
	shutdownTimeout := time.Duration(cfg.Int64(SERVER_SHUTDOWN_TIMEOUT)) * time.Second
	shutdownErr := handleShutdown(context.Background(), srv, shutdownTimeout)

	if listenAndServeError != nil {
		// If ListenAndServe failed, that's the primary error to return.
		if shutdownErr != nil {
			log.Error().Err(shutdownErr).Msg("additional error during shutdown attempt after ListenAndServe failure.")
		}
		return listenAndServeError
	}

	// If ListenAndServe did not error out (i.e., it was http.ErrServerClosed or a signal was received),
	// then the result of the shutdown attempt is the final outcome.
	return shutdownErr
}
