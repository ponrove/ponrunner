package ponrunner

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ponrove/configura"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	otelglobal "go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

// TestMain can be used for global setup/teardown.
func TestMain(m *testing.M) {
	// Optional: To globally silence slog for all tests unless a test overrides it:
	// originalDefault := slog.Default()
	// slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	// defer slog.SetDefault(originalDefault)
	code := m.Run()
	os.Exit(code)
}

func TestSetupOTelSDK_Disabled(t *testing.T) {
	ctx := context.Background()
	emptyCfg := configura.NewConfigImpl()
	err := configura.WriteConfiguration(emptyCfg, map[configura.Variable[bool]]bool{
		OTEL_ENABLED: false,
	})
	require.NoError(t, err, "Failed to write free port to configuration")
	finalCfg := configura.Merge(newDefaultCfg(), emptyCfg)

	originalSlogLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(originalSlogLogger)

	originalTracerProvider := otel.GetTracerProvider()
	originalMeterProvider := otel.GetMeterProvider()
	originalLoggerProvider := otelglobal.GetLoggerProvider()

	shutdown, err := setupOTelSDK(ctx, finalCfg)
	require.NoError(t, err, "setupOTelSDK should not return an error when OTel is disabled")
	require.Nil(t, shutdown, "shutdown function should be nil when OTel is disabled")

	assert.Equal(t, originalTracerProvider, otel.GetTracerProvider(), "TracerProvider should not have been changed")
	assert.Equal(t, originalMeterProvider, otel.GetMeterProvider(), "MeterProvider should not have been changed")
	assert.Equal(t, originalLoggerProvider, otelglobal.GetLoggerProvider(), "LoggerProvider should not have been changed")
}

func TestSetupOTelSDK_Enabled_DefaultServiceName(t *testing.T) {
	ctx := context.Background()
	emptyCfg := configura.NewConfigImpl()
	err := configura.WriteConfiguration(emptyCfg, map[configura.Variable[bool]]bool{
		OTEL_ENABLED:         true,
		OTEL_TRACES_ENABLED:  true,
		OTEL_METRICS_ENABLED: true,
		OTEL_LOGS_ENABLED:    true,
	})
	require.NoError(t, err, "Failed to write free port to configuration")
	finalCfg := configura.Merge(newDefaultCfg(), emptyCfg)

	originalSlogLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(originalSlogLogger) // Restored after OTel's potential override

	originalTracerProvider := otel.GetTracerProvider()
	originalMeterProvider := otel.GetMeterProvider()
	originalLoggerProvider := otelglobal.GetLoggerProvider()
	defer func() {
		otel.SetTracerProvider(originalTracerProvider)
		otel.SetMeterProvider(originalMeterProvider)
		otelglobal.SetLoggerProvider(originalLoggerProvider)
		slog.SetDefault(originalSlogLogger) // Ensure slog is restored
	}()

	shutdown, err := setupOTelSDK(ctx, finalCfg)
	require.NoError(t, err, "setupOTelSDK should succeed when OTel is enabled")
	require.NotNil(t, shutdown, "shutdown function should be non-nil")

	assert.NotEqual(t, originalTracerProvider, otel.GetTracerProvider(), "TracerProvider should have been updated")
	assert.NotEqual(t, originalMeterProvider, otel.GetMeterProvider(), "MeterProvider should have been updated")
	assert.NotEqual(t, originalLoggerProvider, otelglobal.GetLoggerProvider(), "LoggerProvider should have been updated")

	err = shutdown(context.Background())
	assert.NoError(t, err, "shutdown function should execute without error")

	err = shutdown(context.Background()) // Call again for idempotency check
	assert.NoError(t, err, "calling shutdown again should not error")
}

func TestSetupOTelSDK_Enabled_CustomServiceName(t *testing.T) {
	ctx := context.Background()
	customServiceName := "my-test-service"

	emptyCfg := configura.NewConfigImpl()
	err := configura.WriteConfiguration(emptyCfg, map[configura.Variable[bool]]bool{
		OTEL_ENABLED:         true,
		OTEL_TRACES_ENABLED:  true,
		OTEL_METRICS_ENABLED: true,
		OTEL_LOGS_ENABLED:    true,
	})
	require.NoError(t, err, "Failed to write free port to configuration")
	err = configura.WriteConfiguration(emptyCfg, map[configura.Variable[string]]string{
		OTEL_SERVICE_NAME: customServiceName,
	})
	finalCfg := configura.Merge(newDefaultCfg(), emptyCfg)

	originalSlogLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(originalSlogLogger)

	originalTracerProvider := otel.GetTracerProvider()
	originalMeterProvider := otel.GetMeterProvider()
	originalLoggerProvider := otelglobal.GetLoggerProvider()
	defer func() {
		otel.SetTracerProvider(originalTracerProvider)
		otel.SetMeterProvider(originalMeterProvider)
		otelglobal.SetLoggerProvider(originalLoggerProvider)
		slog.SetDefault(originalSlogLogger)
	}()

	shutdown, err := setupOTelSDK(ctx, finalCfg)
	require.NoError(t, err, "setupOTelSDK should succeed with custom service name")
	require.NotNil(t, shutdown)

	assert.NotEqual(t, originalTracerProvider, otel.GetTracerProvider(), "TracerProvider should have been updated")
	assert.NotEqual(t, originalMeterProvider, otel.GetMeterProvider(), "MeterProvider should have been updated")
	assert.NotEqual(t, originalLoggerProvider, otelglobal.GetLoggerProvider(), "LoggerProvider should have been updated")

	err = shutdown(context.Background())
	assert.NoError(t, err, "shutdown function should execute without error for custom service name")
}

func TestNewTracerProvider_Success(t *testing.T) {
	cfg := newDefaultCfg() // Defaults to stdout exporter
	ctx := context.Background()
	res, errRes := sdkresource.New(ctx, sdkresource.WithAttributes(semconv.ServiceName("test-tracer-service")))
	require.NoError(t, errRes)

	originalSlogLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(originalSlogLogger)

	tp, err := newTracerProvider(ctx, res, cfg)
	require.NoError(t, err, "newTracerProvider should succeed")
	require.NotNil(t, tp, "TracerProvider should not be nil")

	err = tp.Shutdown(context.Background())
	assert.NoError(t, err, "TracerProvider shutdown should succeed")
}

func TestNewMeterProvider_Success(t *testing.T) {
	cfg := newDefaultCfg() // Defaults to stdout exporter
	ctx := context.Background()
	res, errRes := sdkresource.New(ctx, sdkresource.WithAttributes(semconv.ServiceName("test-meter-service")))
	require.NoError(t, errRes)

	originalSlogLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(originalSlogLogger)

	mp, err := newMeterProvider(ctx, res, cfg)
	require.NoError(t, err, "newMeterProvider should succeed")
	require.NotNil(t, mp, "MeterProvider should not be nil")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = mp.Shutdown(shutdownCtx)
	assert.NoError(t, err, "MeterProvider shutdown should succeed")
}

func TestNewLoggerProvider_Success(t *testing.T) {
	cfg := newDefaultCfg() // Defaults to stdout exporter
	ctx := context.Background()
	res, errRes := sdkresource.New(ctx, sdkresource.WithAttributes(semconv.ServiceName("test-logger-service")))
	require.NoError(t, errRes)

	logOutput := &MemoryWriter{} // Still need this for the slog redirection check
	originalSlogLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(originalSlogLogger) // Restore original at the very end

	originalOtelGlobalLP := otelglobal.GetLoggerProvider()
	defer otelglobal.SetLoggerProvider(originalOtelGlobalLP) // Restore OTel global LP

	lp, err := newLoggerProvider(ctx, res, cfg)
	require.NoError(t, err, "newLoggerProvider should succeed")
	require.NotNil(t, lp, "LoggerProvider should not be nil")

	// Verify that the default slog logger was indeed changed by newLoggerProvider.
	logOutput.Reset()
	slog.InfoContext(ctx, "Test message after newLoggerProvider slog.SetDefault")
	assert.NotContains(t, logOutput.String(), "Test message after newLoggerProvider slog.SetDefault",
		"Log message after newLoggerProvider's SetDefault should not be in old MemoryWriter because slog was reconfigured")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = lp.Shutdown(shutdownCtx)
	assert.NoError(t, err, "LoggerProvider shutdown should succeed")
}

func TestSetupOTelSDK_FeaturesDisabled(t *testing.T) {
	tests := []struct {
		name           string
		tracesEnabled  bool
		metricsEnabled bool
		logsEnabled    bool
	}{
		{name: "All features enabled", tracesEnabled: true, metricsEnabled: true, logsEnabled: true},
		{name: "Traces disabled", tracesEnabled: false, metricsEnabled: true, logsEnabled: true},
		{name: "Metrics disabled", tracesEnabled: true, metricsEnabled: false, logsEnabled: true},
		{name: "Logs disabled", tracesEnabled: true, metricsEnabled: true, logsEnabled: false},
		{name: "All features disabled", tracesEnabled: false, metricsEnabled: false, logsEnabled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			emptyCfg := configura.NewConfigImpl()
			err := configura.WriteConfiguration(emptyCfg, map[configura.Variable[bool]]bool{
				OTEL_ENABLED:         true,
				OTEL_TRACES_ENABLED:  tt.tracesEnabled,
				OTEL_METRICS_ENABLED: tt.metricsEnabled,
				OTEL_LOGS_ENABLED:    tt.logsEnabled,
			})
			require.NoError(t, err, "Failed to write free port to configuration")
			finalCfg := configura.Merge(newDefaultCfg(), emptyCfg)

			originalSlogLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})))
			defer slog.SetDefault(originalSlogLogger)

			initialTracerProvider := otel.GetTracerProvider()
			initialMeterProvider := otel.GetMeterProvider()
			initialLoggerProvider := otelglobal.GetLoggerProvider()

			defer func() {
				otel.SetTracerProvider(initialTracerProvider)
				otel.SetMeterProvider(initialMeterProvider)
				otelglobal.SetLoggerProvider(initialLoggerProvider)
				slog.SetDefault(originalSlogLogger)
			}()

			shutdown, err := setupOTelSDK(ctx, finalCfg)
			require.NoError(t, err)
			defer shutdown(context.Background())

			currentTracerProvider := otel.GetTracerProvider()
			currentMeterProvider := otel.GetMeterProvider()
			currentLoggerProvider := otelglobal.GetLoggerProvider()

			if tt.tracesEnabled {
				assert.NotEqual(t, initialTracerProvider, currentTracerProvider, "TracerProvider should have changed when traces are enabled")
				_, ok := currentTracerProvider.(*sdktrace.TracerProvider)
				assert.True(t, ok, "Expected SDK TracerProvider when traces are enabled")
			} else {
				assert.Equal(t, initialTracerProvider, currentTracerProvider, "TracerProvider should NOT have changed when traces are disabled")
			}

			if tt.metricsEnabled {
				assert.NotEqual(t, initialMeterProvider, currentMeterProvider, "MeterProvider should have changed when metrics are enabled")
				_, ok := currentMeterProvider.(*sdkmetric.MeterProvider)
				assert.True(t, ok, "Expected SDK MeterProvider when metrics are enabled")
			} else {
				assert.Equal(t, initialMeterProvider, currentMeterProvider, "MeterProvider should NOT have changed when metrics are disabled")
			}

			if tt.logsEnabled {
				assert.NotEqual(t, initialLoggerProvider, currentLoggerProvider, "LoggerProvider should have changed when logs are enabled")
				_, ok := currentLoggerProvider.(*sdklog.LoggerProvider)
				assert.True(t, ok, "Expected SDK LoggerProvider when logs are enabled")
			} else {
				assert.Equal(t, initialLoggerProvider, currentLoggerProvider, "LoggerProvider should NOT have changed when logs are disabled")
			}
		})
	}
}

// startMockGRPCServer starts a minimal gRPC server on a random port.
func startMockGRPCServer(t *testing.T) (addr string, stop func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err, "Failed to listen on a port for mock gRPC server")

	s := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	// No services registered, just needs to accept connections for exporter init/shutdown.
	go func() {
		if errS := s.Serve(lis); errS != nil && errS != grpc.ErrServerStopped {
			t.Logf("Mock gRPC server failed: %v", errS)
		}
	}()
	return lis.Addr().String(), func() {
		s.GracefulStop()
	}
}

// startMockHTTPServer starts an httptest server for OTLP/HTTP.
func startMockHTTPServer(t *testing.T) (url string, stop func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// For testing, we just need to acknowledge the request.
		// OTLP paths: /v1/traces, /v1/metrics, /v1/logs
		w.WriteHeader(http.StatusOK)
	}))
	return server.URL, func() {
		server.Close()
	}
}
