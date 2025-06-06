package ponrunner

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

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

// TestMain can be used for global setup/teardown.
func TestMain(m *testing.M) {
	// To globally silence slog for all tests unless a test overrides it:
	// originalDefault := slog.Default()
	// slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	// defer slog.SetDefault(originalDefault)
	code := m.Run()
	os.Exit(code)
}

func TestSetupOTelSDK_Disabled(t *testing.T) {
	ctx := context.Background()
	cfg := newDefaultCfg()
	cfg.RegBool[OTEL_ENABLED] = false

	logOutput := &MemoryWriter{}
	originalSlogLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(originalSlogLogger)

	shutdown, err := setupOTelSDK(ctx, cfg)
	require.NoError(t, err, "setupOTelSDK should not return an error when OTel is disabled")
	require.NotNil(t, shutdown, "shutdown function should be non-nil (no-op) when OTel is disabled")

	logs := logOutput.String()
	assert.Contains(t, logs, "OpenTelemetry is disabled via OTEL_ENABLED. Skipping SDK setup.", "Log should indicate OTel is disabled")

	logOutput.Reset()
	err = shutdown(context.Background()) // Use a fresh background context for shutdown
	assert.NoError(t, err, "no-op shutdown function should not return an error")

	shutdownLogs := logOutput.String()
	assert.Contains(t, shutdownLogs, "OpenTelemetry was disabled; no-op shutdown called.", "Log should indicate no-op shutdown was called")
}

func TestSetupOTelSDK_Enabled_DefaultServiceName(t *testing.T) {
	ctx := context.Background()
	cfg := newDefaultCfg()
	cfg.RegBool[OTEL_ENABLED] = true
	cfg.RegBool[OTEL_TRACES_ENABLED] = true
	cfg.RegBool[OTEL_METRICS_ENABLED] = true
	cfg.RegBool[OTEL_LOGS_ENABLED] = true

	logOutput := &MemoryWriter{}
	originalSlogLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(originalSlogLogger)

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
	assert.Contains(t, logs, "OpenTelemetry propagator set up.")
	assert.Contains(t, logs, "OpenTelemetry tracer provider set up") // This is a partial match, still okay
	assert.Contains(t, logs, "OpenTelemetry meter provider set up")  // This is a partial match, still okay
	// This log message is from newLoggerProvider, before it calls slog.SetDefault.
	assert.Contains(t, logs, "OTel SDK LoggerProvider created.")

	// The following logs occur *after* newLoggerProvider calls slog.SetDefault.
	// Thus, they are routed through the OTel pipeline (to os.Stdout by default)
	// and will NOT be captured by `logOutput`.
	// assert.Contains(t, logs, "Default slog logger replaced...")
	// assert.Contains(t, logs, "OpenTelemetry logger provider configured for OTel SDK and slog.")
	// assert.Contains(t, logs, "OpenTelemetry SDK setup completed successfully")

	assert.NotEqual(t, originalTracerProvider, otel.GetTracerProvider(), "TracerProvider should have been updated")
	assert.NotEqual(t, originalMeterProvider, otel.GetMeterProvider(), "MeterProvider should have been updated")
	assert.NotEqual(t, originalLoggerProvider, otelglobal.GetLoggerProvider(), "LoggerProvider should have been updated")

	logOutput.Reset()
	// Shutdown logs also go through the (now OTel-bridged) slog default logger.
	err = shutdown(context.Background())
	assert.NoError(t, err, "shutdown function should execute without error")
	// shutdownLogs := logOutput.String()
	// assert.Contains(t, shutdownLogs, "OpenTelemetry shutdown completed successfully") // This would not be captured by logOutput

	logOutput.Reset()
	err = shutdown(context.Background()) // Call again for idempotency check
	assert.NoError(t, err, "calling shutdown again should not error")
	// idempotentShutdownLogs := logOutput.String()
	// assert.Contains(t, idempotentShutdownLogs, "OpenTelemetry shutdown completed successfully") // Not captured
}

func TestSetupOTelSDK_Enabled_CustomServiceName(t *testing.T) {
	ctx := context.Background()
	cfg := newDefaultCfg()
	cfg.RegBool[OTEL_ENABLED] = true
	customServiceName := "my-test-service"
	cfg.RegString[OTEL_SERVICE_NAME] = customServiceName

	logOutput := &MemoryWriter{}
	originalSlogLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(originalSlogLogger)

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
	assert.Contains(t, logs, "OpenTelemetry is enabled. Proceeding with SDK setup.")
	// Further detailed log assertions are subject to the redirection issue.

	err = shutdown(context.Background())
	assert.NoError(t, err, "shutdown function should execute without error for custom service name")
}

func TestNewTracerProvider_Success(t *testing.T) {
	ctx := context.Background()
	res, errRes := resource.New(ctx, resource.WithAttributes(semconv.ServiceName("test-tracer-service")))
	require.NoError(t, errRes)

	logOutput := &MemoryWriter{}
	originalSlogLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(originalSlogLogger)

	tp, err := newTracerProvider(ctx, res)
	require.NoError(t, err, "newTracerProvider should succeed")
	require.NotNil(t, tp, "TracerProvider should not be nil")

	logs := logOutput.String()
	assert.Contains(t, logs, "Creating stdout trace exporter.")
	assert.Contains(t, logs, "Stdout trace exporter created.")
	assert.Contains(t, logs, "Tracer provider created.")

	err = tp.Shutdown(context.Background())
	assert.NoError(t, err, "TracerProvider shutdown should succeed")
}

func TestNewMeterProvider_Success(t *testing.T) {
	ctx := context.Background()
	res, errRes := resource.New(ctx, resource.WithAttributes(semconv.ServiceName("test-meter-service")))
	require.NoError(t, errRes)

	logOutput := &MemoryWriter{}
	originalSlogLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(originalSlogLogger)

	mp, err := newMeterProvider(ctx, res)
	require.NoError(t, err, "newMeterProvider should succeed")
	require.NotNil(t, mp, "MeterProvider should not be nil")

	logs := logOutput.String()
	assert.Contains(t, logs, "Creating stdout metric exporter.")
	assert.Contains(t, logs, "Stdout metric exporter created.")
	assert.Contains(t, logs, "Meter provider created.")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = mp.Shutdown(shutdownCtx)
	assert.NoError(t, err, "MeterProvider shutdown should succeed")
}

func TestNewLoggerProvider_Success(t *testing.T) {
	ctx := context.Background()
	res, errRes := resource.New(ctx, resource.WithAttributes(semconv.ServiceName("test-logger-service")))
	require.NoError(t, errRes)

	logOutput := &MemoryWriter{}
	originalSlogLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))
	// Defer restoring the original slog logger. This will run after the OTel global LP is restored.
	defer slog.SetDefault(originalSlogLogger)

	originalOtelGlobalLP := otelglobal.GetLoggerProvider()
	defer otelglobal.SetLoggerProvider(originalOtelGlobalLP)

	lp, err := newLoggerProvider(ctx, res)
	require.NoError(t, err, "newLoggerProvider should succeed")
	require.NotNil(t, lp, "LoggerProvider should not be nil")

	logs := logOutput.String() // Logs captured *before* newLoggerProvider calls slog.SetDefault
	assert.Contains(t, logs, "Creating OTel stdout log exporter.")
	assert.Contains(t, logs, "OTel stdout log exporter created.")
	assert.Contains(t, logs, "Creating OTel SDK LoggerProvider.")
	assert.Contains(t, logs, "OTel SDK LoggerProvider created.")

	// This log is issued *after* newLoggerProvider calls slog.SetDefault itself.
	// So it goes to the OTel pipeline (os.Stdout) and not our MemoryWriter.
	// assert.Contains(t, logs, "Default slog logger replaced. Application logs via slog will now be processed by the OTel logging pipeline.")

	// Verify that the default slog logger was indeed changed by newLoggerProvider.
	// After newLoggerProvider, logs should go to the OTel pipeline, not our MemoryWriter.
	logOutput.Reset()
	slog.InfoContext(ctx, "Test message after newLoggerProvider slog.SetDefault")
	assert.NotContains(t, logOutput.String(), "Test message after newLoggerProvider slog.SetDefault",
		"Log message after newLoggerProvider's SetDefault should not be in old MemoryWriter")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = lp.Shutdown(shutdownCtx)
	assert.NoError(t, err, "LoggerProvider shutdown should succeed")
}

func TestSetupOTelSDK_FeaturesDisabled(t *testing.T) {
	tests := []struct {
		name                           string
		tracesEnabled                  bool
		metricsEnabled                 bool
		logsEnabled                    bool
		expectTracerSetupLog           bool // True if "OpenTelemetry tracer provider set up" is expected
		expectMeterSetupLog            bool // True if "OpenTelemetry meter provider set up" is expected
		expectLoggerProviderCreatedLog bool // True if "OTel SDK LoggerProvider created" is expected (from newLoggerProvider)
		expectedTracesDisabledLog      string
		expectedMetricsDisabledLog     string
		expectedLogsDisabledLog        string
	}{
		{
			name:          "All features enabled",
			tracesEnabled: true, metricsEnabled: true, logsEnabled: true,
			expectTracerSetupLog: true, expectMeterSetupLog: true, expectLoggerProviderCreatedLog: true,
		},
		{
			name:          "Traces disabled",
			tracesEnabled: false, metricsEnabled: true, logsEnabled: true,
			expectTracerSetupLog: false, expectMeterSetupLog: true, expectLoggerProviderCreatedLog: true,
			expectedTracesDisabledLog: "OpenTelemetry tracing is disabled via OTEL_TRACES_ENABLED. Skipping tracer provider setup.",
		},
		{
			name:          "Metrics disabled",
			tracesEnabled: true, metricsEnabled: false, logsEnabled: true,
			expectTracerSetupLog: true, expectMeterSetupLog: false, expectLoggerProviderCreatedLog: true,
			expectedMetricsDisabledLog: "OpenTelemetry metrics are disabled via OTEL_METRICS_ENABLED. Skipping meter provider setup.",
		},
		{
			name:          "Logs disabled",
			tracesEnabled: true, metricsEnabled: true, logsEnabled: false,
			expectTracerSetupLog: true, expectMeterSetupLog: true, expectLoggerProviderCreatedLog: false, // newLoggerProvider won't be called
			expectedLogsDisabledLog: "OpenTelemetry logging is disabled via OTEL_LOGS_ENABLED. Skipping logger provider setup.",
		},
		{
			name:          "All features disabled",
			tracesEnabled: false, metricsEnabled: false, logsEnabled: false,
			expectTracerSetupLog: false, expectMeterSetupLog: false, expectLoggerProviderCreatedLog: false,
			expectedTracesDisabledLog:  "OpenTelemetry tracing is disabled via OTEL_TRACES_ENABLED. Skipping tracer provider setup.",
			expectedMetricsDisabledLog: "OpenTelemetry metrics are disabled via OTEL_METRICS_ENABLED. Skipping meter provider setup.",
			expectedLogsDisabledLog:    "OpenTelemetry logging is disabled via OTEL_LOGS_ENABLED. Skipping logger provider setup.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := newDefaultCfg()
			cfg.RegBool[OTEL_ENABLED] = true
			cfg.RegBool[OTEL_TRACES_ENABLED] = tt.tracesEnabled
			cfg.RegBool[OTEL_METRICS_ENABLED] = tt.metricsEnabled
			cfg.RegBool[OTEL_LOGS_ENABLED] = tt.logsEnabled

			logOutput := &MemoryWriter{}
			originalSlogLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelInfo})))
			defer slog.SetDefault(originalSlogLogger)

			// Manage OTel global providers
			originalTracerProvider := otel.GetTracerProvider()
			originalMeterProvider := otel.GetMeterProvider()
			originalLoggerProvider := otelglobal.GetLoggerProvider()
			defer func() {
				otel.SetTracerProvider(originalTracerProvider)
				otel.SetMeterProvider(originalMeterProvider)
				otelglobal.SetLoggerProvider(originalLoggerProvider)
			}()

			shutdown, err := setupOTelSDK(ctx, cfg)
			require.NoError(t, err)
			defer shutdown(context.Background())

			logs := logOutput.String()

			if tt.expectTracerSetupLog {
				assert.Contains(t, logs, "OpenTelemetry tracer provider set up")
			} else {
				assert.NotContains(t, logs, "OpenTelemetry tracer provider set up")
				if tt.expectedTracesDisabledLog != "" {
					assert.Contains(t, logs, tt.expectedTracesDisabledLog)
				}
			}

			if tt.expectMeterSetupLog {
				assert.Contains(t, logs, "OpenTelemetry meter provider set up")
			} else {
				assert.NotContains(t, logs, "OpenTelemetry meter provider set up")
				if tt.expectedMetricsDisabledLog != "" {
					assert.Contains(t, logs, tt.expectedMetricsDisabledLog)
				}
			}

			// Check for "OTel SDK LoggerProvider created" which is from newLoggerProvider (if called)
			// OR the "logging is disabled" message from setupOTelSDK.
			if tt.expectLoggerProviderCreatedLog { // Implies OTEL_LOGS_ENABLED = true
				assert.Contains(t, logs, "OTel SDK LoggerProvider created.")
				// The log "OpenTelemetry logger provider configured..." from setupOTelSDK is not captured by logOutput.
			} else { // Implies OTEL_LOGS_ENABLED = false
				assert.NotContains(t, logs, "OTel SDK LoggerProvider created.")
				assert.NotContains(t, logs, "OpenTelemetry logger provider configured for OTel SDK and slog.")
				if tt.expectedLogsDisabledLog != "" {
					assert.Contains(t, logs, tt.expectedLogsDisabledLog)
				}
			}
		})
	}
}
