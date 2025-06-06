package ponrunner

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/ponrove/configura"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newDefaultCfg() *configura.ConfigImpl {
	cfg := configura.NewConfigImpl()
	cfg.RegInt64[SERVER_PORT] = 8080
	cfg.RegInt64[SERVER_REQUEST_TIMEOUT] = 1
	cfg.RegInt64[SERVER_SHUTDOWN_TIMEOUT] = 1
	cfg.RegInt64[SERVER_READ_TIMEOUT] = 1
	cfg.RegInt64[SERVER_WRITE_TIMEOUT] = 1
	cfg.RegString[SERVER_OPENFEATURE_PROVIDER_NAME] = "go-feature-flag"
	cfg.RegString[SERVER_OPENFEATURE_PROVIDER_URL] = "http://localhost:8080"
	cfg.RegBool[OTEL_ENABLED] = false
	cfg.RegBool[OTEL_LOGS_ENABLED] = true
	cfg.RegBool[OTEL_METRICS_ENABLED] = true
	cfg.RegBool[OTEL_TRACES_ENABLED] = true
	cfg.RegString[OTEL_SERVICE_NAME] = "ponrunner-test"
	cfg.RegString[SERVER_LOG_LEVEL] = "debug"
	cfg.RegString[SERVER_LOG_FORMAT] = "json"
	return cfg
}

// MockServerControl is a mock type for the serverControl interface
type MockServerControl struct {
	mock.Mock
}

// Shutdown is a mock method for Shutdown
func (m *MockServerControl) Shutdown(ctx context.Context) error {
	args := m.Called(ctx)
	// Simulate context deadline respected by Shutdown
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return args.Error(0)
}

func TestHandleShutdown_Successful(t *testing.T) {
	mockSrv := new(MockServerControl)
	ctx := context.Background()
	shutdownTimeout := 100 * time.Millisecond

	// Expect Shutdown to be called with a context that has a deadline derived from shutdownTimeout
	mockSrv.On("Shutdown", mock.AnythingOfType("*context.timerCtx")).Return(nil).Once()

	err := handleServerShutdown(ctx, mockSrv, shutdownTimeout)

	assert.NoError(t, err)
	mockSrv.AssertExpectations(t)
}

func TestHandleShutdown_ShutdownError(t *testing.T) {
	mockSrv := new(MockServerControl)
	ctx := context.Background()
	shutdownTimeout := 100 * time.Millisecond
	expectedErr := errors.New("shutdown failed")

	mockSrv.On("Shutdown", mock.AnythingOfType("*context.timerCtx")).Return(expectedErr).Once()

	err := handleServerShutdown(ctx, mockSrv, shutdownTimeout)

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	mockSrv.AssertExpectations(t)
}

func TestHandleShutdown_AlreadyClosed(t *testing.T) {
	mockSrv := new(MockServerControl)
	ctx := context.Background()
	shutdownTimeout := 100 * time.Millisecond

	mockSrv.On("Shutdown", mock.AnythingOfType("*context.timerCtx")).Return(http.ErrServerClosed).Once()

	err := handleServerShutdown(ctx, mockSrv, shutdownTimeout)

	assert.NoError(t, err) // ErrServerClosed is treated as a successful shutdown
	mockSrv.AssertExpectations(t)
}

func TestHandleShutdown_Timeout(t *testing.T) {
	mockSrv := new(MockServerControl)
	ctx := context.Background()
	shutdownTimeout := 50 * time.Millisecond // Short timeout for the test

	// Simulate Shutdown taking longer than the timeout
	mockSrv.On("Shutdown", mock.AnythingOfType("*context.timerCtx")).
		Run(func(args mock.Arguments) {
			// Wait for the context passed to Shutdown to be canceled by the timeout in handleShutdown
			ctxArg := args.Get(0).(context.Context)
			<-ctxArg.Done()
		}).
		Return(context.DeadlineExceeded). // http.Server.Shutdown returns ctx.Err() when its context is done.
		Once()

	err := handleServerShutdown(ctx, mockSrv, shutdownTimeout)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "Expected context.DeadlineExceeded, got %v", err)
	mockSrv.AssertExpectations(t)
}

func TestHandleShutdown_ParentContextCancellation(t *testing.T) {
	mockSrv := new(MockServerControl)
	// Parent context that we will cancel
	parentCtx, cancelParent := context.WithCancel(context.Background())
	shutdownTimeout := 200 * time.Millisecond // Ample time, cancellation should be quicker

	// Expect Shutdown to be called. The context it receives (shutdownOpCtx) will be a child of parentCtx.
	mockSrv.On("Shutdown", mock.AnythingOfType("*context.timerCtx")).
		Run(func(args mock.Arguments) {
			ctxArg := args.Get(0).(context.Context) // This is shutdownOpCtx
			// Wait until this context is canceled. In this test, it will be due to parentCtx cancellation.
			<-ctxArg.Done()
		}).
		Return(context.Canceled). // When the context passed to Shutdown is canceled.
		Once()

	// Cancel the parent context shortly after calling handleShutdown
	go func() {
		time.Sleep(50 * time.Millisecond) // Allow handleShutdown to set up its own context
		cancelParent()                    // This cancellation should propagate to shutdownOpCtx
	}()

	err := handleServerShutdown(parentCtx, mockSrv, shutdownTimeout)

	assert.Error(t, err)
	// The error returned by srv.Shutdown when its context is canceled is context.Canceled or context.DeadlineExceeded.
	// Since we trigger parent cancellation, and our mock returns context.Canceled, we expect that.
	assert.True(t, errors.Is(err, context.Canceled), "Expected context.Canceled, got %v", err)
	mockSrv.AssertExpectations(t)
}

func TestHandleShutdown_ZeroShutdownTimeout(t *testing.T) {
	mockSrv := new(MockServerControl)
	ctx := context.Background()
	shutdownTimeout := 0 * time.Millisecond // Zero timeout

	// With a zero timeout, context.WithTimeout creates an already-canceled context.
	// So, Shutdown should be called with an already-canceled context.
	mockSrv.On("Shutdown", mock.AnythingOfType("*context.timerCtx")).
		Run(func(args mock.Arguments) {
			ctxArg := args.Get(0).(context.Context)
			assert.Error(t, ctxArg.Err(), "Context passed to Shutdown should be already canceled")
			assert.True(t, errors.Is(ctxArg.Err(), context.DeadlineExceeded) || errors.Is(ctxArg.Err(), context.Canceled), "Context error should be DeadlineExceeded or Canceled")
		}).
		Return(context.DeadlineExceeded). // http.Server.Shutdown would return ctx.Err().
		Once()

	err := handleServerShutdown(ctx, mockSrv, shutdownTimeout)

	assert.Error(t, err)
	// Expect DeadlineExceeded because WithTimeout(parent, 0) results in a context that is immediately done with DeadlineExceeded.
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "Expected context.DeadlineExceeded for zero timeout, got %v", err)
	mockSrv.AssertExpectations(t)
}

// Helper function to get a free port
func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func TestStart_SuccessfulShutdown(t *testing.T) {
	t.Parallel()

	cfg := newDefaultCfg()
	freePort, err := getFreePort()
	require.NoError(t, err, "Failed to get free port")
	cfg.RegInt64[SERVER_PORT] = int64(freePort) // Use a free port

	ctx, cancel := context.WithCancel(context.Background())
	startErrChan := make(chan error, 1)

	r := chi.NewRouter()
	go func() {
		// We expect Start to return nil on successful graceful shutdown.
		startErrChan <- Start(ctx, cfg, r, func(cfg configura.Config, r chi.Router, a huma.API) error {
			return nil
		})
	}()

	// Wait for the server to start by trying to connect
	serverAddr := fmt.Sprintf("localhost:%d", freePort)
	connectionSuccessful := false
	for i := 0; i < 40; i++ { // Retry for up to 2 seconds
		conn, dialErr := net.DialTimeout("tcp", serverAddr, 50*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			connectionSuccessful = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, connectionSuccessful, "Server did not start listening on %s", serverAddr)

	// Simulate a shutdown signal by cancelling the context
	cancel()

	// Wait for Start to exit
	select {
	case err := <-startErrChan:
		// On graceful shutdown triggered by context cancellation, Start should return nil.
		// http.ErrServerClosed is handled internally.
		assert.NoError(t, err, "Start should exit gracefully without error")
	case <-time.After(3 * time.Second): // Generous timeout for shutdown
		t.Fatal("Start did not exit after context cancellation")
	}
}

func TestStart_ListenAndServeFails(t *testing.T) {
	t.Parallel()

	// Create a listener to occupy a port
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "Failed to create a listener for busy port")
	busyPort := listener.Addr().(*net.TCPAddr).Port
	defer listener.Close() // Keep the port busy for the duration of the test

	log.Debug().Msgf("port: %v", busyPort)

	cfg := newDefaultCfg()
	cfg.RegInt64[SERVER_PORT] = int64(busyPort) // Use a free port

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second) // Test timeout
	defer cancel()

	r := chi.NewRouter()
	runErr := Start(ctx, cfg, r, func(c configura.Config, r chi.Router, a huma.API) error { return nil })

	assert.Error(t, runErr, "Start should return an error if ListenAndServe fails")
	// Check if the error is a bind error (OS-dependent message)
	// Example: "bind: address already in use" or "listen tcp :<port>: bind: address already in use"
	// net.OpError is common for such issues.
	var opError *net.OpError
	if errors.As(runErr, &opError) {
		assert.Contains(t, strings.ToLower(opError.Err.Error()), "address already in use", "Error message should indicate address in use")
	} else {
		t.Logf("ListenAndServe failed with: %v (type: %T), expected a net.OpError related to binding", runErr, runErr)
		// Fallback to a general check if not OpError but still an error
		assert.Contains(t, strings.ToLower(runErr.Error()), "bind", "Error message should be related to bind failure")
	}
}

func TestStart_APIBundleRegistrationFails(t *testing.T) {
	t.Parallel()
	const unset_fake_configuration_flag configura.Variable[bool] = "unset_fake_configuration_flag"

	testBundle := func(cfg configura.Config, api huma.API) error {
		return cfg.ConfigurationKeysRegistered(unset_fake_configuration_flag)
	}

	cfg := newDefaultCfg()
	freePort, err := getFreePort()
	require.NoError(t, err, "Failed to get free port")

	cfg.RegInt64[SERVER_PORT] = int64(freePort) // Use a free port

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // Test timeout
	defer cancel()

	r := chi.NewRouter()
	runErr := Start(ctx, cfg, r, func(cfg configura.Config, r chi.Router, a huma.API) error {
		return RegisterAPIBundles(cfg, a, testBundle)
	})

	assert.Error(t, runErr)
	assert.True(t, errors.Is(runErr, configura.ErrMissingVariable), "Start should return the error from API bundle registration. Got: %v", runErr)
}
