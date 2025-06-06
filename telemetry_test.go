package ponrunner

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log" // Ensure main log package is imported
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	otelglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// MemoryWriter captures log output for assertions.
type MemoryWriter struct {
	buf strings.Builder
}

// Write implements io.Writer.
func (mw *MemoryWriter) Write(p []byte) (n int, err error) {
	return mw.buf.Write(p)
}

// String returns the captured log output.
func (mw *MemoryWriter) String() string {
	return mw.buf.String()
}

// Reset clears the captured log output.
func (mw *MemoryWriter) Reset() {
	mw.buf.Reset()
}

// TestMain sets up a NopLogger for cleaner test output, individual tests can override.
func TestMain(m *testing.M) {
	originalLogger := log.Logger
	log.Logger = zerolog.Nop() // Suppress global logs

	code := m.Run()

	log.Logger = originalLogger // Restore original logger
	os.Exit(code)
}

func TestSetupOTelSDK_Disabled(t *testing.T) {
	ctx := context.Background()
	cfg := newDefaultCfg()
	cfg.RegBool[OTEL_ENABLED] = false

	logOutput := &MemoryWriter{}
	testLogger := zerolog.New(logOutput).With().Timestamp().Logger()
	originalGlobalLogger := log.Logger
	log.Logger = testLogger
	defer func() { log.Logger = originalGlobalLogger }()

	shutdown, err := setupOTelSDK(ctx, cfg)
	require.NoError(t, err, "setupOTelSDK should not return an error when OTel is disabled")
	require.NotNil(t, shutdown, "shutdown function should be non-nil (no-op) when OTel is disabled")

	logs := logOutput.String()
	assert.Contains(t, logs, "OpenTelemetry is disabled via OTEL_ENABLED. Skipping SDK setup.", "Log should indicate OTel is disabled")

	logOutput.Reset()
	err = shutdown(ctx)
	assert.NoError(t, err, "no-op shutdown function should not return an error")

	shutdownLogs := logOutput.String()
	assert.Contains(t, shutdownLogs, "OpenTelemetry was disabled; no-op shutdown called.", "Log should indicate no-op shutdown was called")
}

func TestSetupOTelSDK_Enabled_DefaultServiceName(t *testing.T) {
	ctx := context.Background()
	cfg := newDefaultCfg()
	cfg.RegBool[OTEL_ENABLED] = true

	logOutput := &MemoryWriter{}
	testLogger := zerolog.New(logOutput).Level(zerolog.InfoLevel)
	originalGlobalLogger := log.Logger
	log.Logger = testLogger
	defer func() { log.Logger = originalGlobalLogger }()

	// Store original providers to restore them later and check if they were changed
	originalTracerProvider := otel.GetTracerProvider()
	originalMeterProvider := otel.GetMeterProvider()
	originalLoggerProvider := otelglobal.GetLoggerProvider()
	defer func() {
		otel.SetTracerProvider(originalTracerProvider)
		otel.SetMeterProvider(originalMeterProvider)
		otelglobal.SetLoggerProvider(originalLoggerProvider)
	}()

	shutdown, err := setupOTelSDK(ctx, cfg)
	require.NoError(t, err, "setupOTelSDK should succeed when OTel is enabled")
	require.NotNil(t, shutdown, "shutdown function should be non-nil")

	logs := logOutput.String()
	assert.Contains(t, logs, "OpenTelemetry is enabled. Proceeding with SDK setup.")
	assert.Contains(t, logs, "OpenTelemetry SDK setup completed successfully")
	// Implicitly tests default service name "ponrove" is used by not setting OTEL_SERVICE_NAME

	assert.NotEqual(t, originalTracerProvider, otel.GetTracerProvider(), "TracerProvider should have been updated")
	assert.NotEqual(t, originalMeterProvider, otel.GetMeterProvider(), "MeterProvider should have been updated")
	assert.NotEqual(t, originalLoggerProvider, otelglobal.GetLoggerProvider(), "LoggerProvider should have been updated")

	logOutput.Reset()
	err = shutdown(ctx)
	assert.NoError(t, err, "shutdown function should execute without error")
	shutdownLogs := logOutput.String()
	assert.Contains(t, shutdownLogs, "OpenTelemetry shutdown completed successfully")

	// Test idempotency of shutdown
	logOutput.Reset()
	err = shutdown(ctx) // Call again
	assert.NoError(t, err, "calling shutdown again should not error")
	// Current implementation logs "OpenTelemetry shutdown completed successfully" again, which is fine.
	// A specific "already shutdown" log isn't strictly necessary.
	assert.Contains(t, logOutput.String(), "OpenTelemetry shutdown completed successfully")
}

func TestSetupOTelSDK_Enabled_CustomServiceName(t *testing.T) {
	ctx := context.Background()
	cfg := newDefaultCfg()
	cfg.RegBool[OTEL_ENABLED] = true
	customServiceName := "my-test-service"
	cfg.RegString[OTEL_SERVICE_NAME] = customServiceName

	logOutput := &MemoryWriter{}
	// Log at debug to catch the service name if we add a specific log for it
	// For now, we'll rely on the semconv.ServiceName in resource creation.
	testLogger := zerolog.New(logOutput).Level(zerolog.InfoLevel)
	originalGlobalLogger := log.Logger
	log.Logger = testLogger
	defer func() { log.Logger = originalGlobalLogger }()

	originalTracerProvider := otel.GetTracerProvider()
	originalMeterProvider := otel.GetMeterProvider()
	originalLoggerProvider := otelglobal.GetLoggerProvider()
	defer func() {
		otel.SetTracerProvider(originalTracerProvider)
		otel.SetMeterProvider(originalMeterProvider)
		otelglobal.SetLoggerProvider(originalLoggerProvider)
	}()

	shutdown, err := setupOTelSDK(ctx, cfg)
	require.NoError(t, err, "setupOTelSDK should succeed with custom service name")
	require.NotNil(t, shutdown)

	logs := logOutput.String()
	assert.Contains(t, logs, "OpenTelemetry SDK setup completed successfully")
	// To directly test the service name, one would need to inspect the Resource
	// associated with the global providers, or add a log line during resource creation
	// in `setupOTelSDK` that explicitly states the service name being used.
	// e.g., log.Debug().Str("service.name", actualServiceName).Msg("Resource created")

	err = shutdown(ctx)
	assert.NoError(t, err, "shutdown function should execute without error for custom service name")
}

func TestNewPropagator(t *testing.T) {
	propagator := newPropagator()
	assert.NotNil(t, propagator, "newPropagator should return a non-nil propagator")
	fields := propagator.Fields()
	assert.Contains(t, fields, "traceparent", "Propagator fields should include traceparent")
	assert.Contains(t, fields, "baggage", "Propagator fields should include baggage")
}

func TestNewTracerProvider_Success(t *testing.T) {
	ctx := context.Background()
	res, errRes := resource.New(ctx, resource.WithAttributes(semconv.ServiceName("test-tracer-service")))
	require.NoError(t, errRes)

	logOutput := &MemoryWriter{}
	testLogger := zerolog.New(logOutput).Level(zerolog.DebugLevel)
	originalGlobalLogger := log.Logger
	log.Logger = testLogger
	defer func() { log.Logger = originalGlobalLogger }()

	tp, err := newTracerProvider(ctx, res)
	require.NoError(t, err, "newTracerProvider should succeed")
	require.NotNil(t, tp, "TracerProvider should not be nil")

	logs := logOutput.String()
	assert.Contains(t, logs, "Initializing stdout trace exporter")
	assert.Contains(t, logs, "Stdout trace exporter initialized")
	assert.Contains(t, logs, "Tracer provider created")

	// Shorten timeout for faster test, default is 5s, we set it to 1s in code.
	err = tp.Shutdown(ctx) // Use a context with timeout if needed, but default should be fine.
	assert.NoError(t, err, "TracerProvider shutdown should succeed")
}

func TestNewMeterProvider_Success(t *testing.T) {
	ctx := context.Background()
	res, errRes := resource.New(ctx, resource.WithAttributes(semconv.ServiceName("test-meter-service")))
	require.NoError(t, errRes)

	logOutput := &MemoryWriter{}
	testLogger := zerolog.New(logOutput).Level(zerolog.DebugLevel)
	originalGlobalLogger := log.Logger
	log.Logger = testLogger
	defer func() { log.Logger = originalGlobalLogger }()

	mp, err := newMeterProvider(ctx, res)
	require.NoError(t, err, "newMeterProvider should succeed")
	require.NotNil(t, mp, "MeterProvider should not be nil")

	logs := logOutput.String()
	assert.Contains(t, logs, "Initializing stdout metric exporter")
	assert.Contains(t, logs, "Stdout metric exporter initialized")
	assert.Contains(t, logs, "Meter provider created")

	// Shutdown can take time due to the periodic reader. Use a context with a timeout for robust testing.
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second) // Default interval is 3s in code.
	defer cancel()
	err = mp.Shutdown(shutdownCtx)
	assert.NoError(t, err, "MeterProvider shutdown should succeed")
}

func TestNewLoggerProvider_Success(t *testing.T) {
	ctx := context.Background()
	res, errRes := resource.New(ctx, resource.WithAttributes(semconv.ServiceName("test-logger-service")))
	require.NoError(t, errRes)

	logOutput := &MemoryWriter{}
	testLogger := zerolog.New(logOutput).Level(zerolog.DebugLevel)
	originalGlobalLogger := log.Logger
	log.Logger = testLogger
	defer func() { log.Logger = originalGlobalLogger }()

	lp, err := newLoggerProvider(ctx, res)
	require.NoError(t, err, "newLoggerProvider should succeed")
	require.NotNil(t, lp, "LoggerProvider should not be nil")

	logs := logOutput.String()
	assert.Contains(t, logs, "Initializing stdout log exporter")
	assert.Contains(t, logs, "Stdout log exporter initialized")
	assert.Contains(t, logs, "Logger provider created")

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second) // Batch processor might take time.
	defer cancel()
	err = lp.Shutdown(shutdownCtx)
	assert.NoError(t, err, "LoggerProvider shutdown should succeed")
}
