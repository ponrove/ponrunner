package ponrunner

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ponrove/configura"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	otelglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0" // Use a specific version
)

const (
	OTEL_ENABLED         configura.Variable[bool]   = "OTEL_ENABLED"
	OTEL_LOGS_ENABLED    configura.Variable[bool]   = "OTEL_LOGS_ENABLED"
	OTEL_METRICS_ENABLED configura.Variable[bool]   = "OTEL_METRICS_ENABLED"
	OTEL_TRACES_ENABLED  configura.Variable[bool]   = "OTEL_TRACES_ENABLED"
	OTEL_SERVICE_NAME    configura.Variable[string] = "OTEL_SERVICE_NAME"
)

// shutdownFunc is a type for functions that perform cleanup.
type shutdownFunc func(context.Context) error

// initializeResource creates a new OpenTelemetry resource.
func initializeResource(ctx context.Context, cfg configura.Config) (*resource.Resource, error) {
	slog.DebugContext(ctx, "Initializing OpenTelemetry resource.")
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(configura.Fallback(cfg.String(OTEL_SERVICE_NAME), "ponrove")),
		),
	)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create OpenTelemetry resource", slog.Any("error", err))
		return nil, err
	}
	slog.InfoContext(ctx, "OpenTelemetry resource initialized.", slog.String("service.name", configura.Fallback(cfg.String(OTEL_SERVICE_NAME), "ponrove")))
	return res, nil
}

// initializePropagator sets up the OpenTelemetry propagator.
func initializePropagator(ctx context.Context) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	slog.InfoContext(ctx, "OpenTelemetry propagator set up.")
}

// initializeTracerProvider sets up the OpenTelemetry tracer provider.
func initializeTracerProvider(ctx context.Context, res *resource.Resource) (*trace.TracerProvider, shutdownFunc, error) {
	slog.DebugContext(ctx, "Attempting to initialize OpenTelemetry tracer provider.")
	tracerProvider, err := newTracerProvider(ctx, res)
	if err != nil {
		// newTracerProvider already logs the specifics of its failure
		slog.ErrorContext(ctx, "Failed to initialize tracer provider", slog.Any("error", err))
		return nil, nil, err
	}

	otel.SetTracerProvider(tracerProvider)
	slog.InfoContext(ctx, "OpenTelemetry tracer provider set up and registered globally.")
	return tracerProvider, tracerProvider.Shutdown, nil
}

// initializeMeterProvider sets up the OpenTelemetry meter provider.
func initializeMeterProvider(ctx context.Context, res *resource.Resource) (*metric.MeterProvider, shutdownFunc, error) {
	slog.DebugContext(ctx, "Attempting to initialize OpenTelemetry meter provider.")
	meterProvider, err := newMeterProvider(ctx, res)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to initialize meter provider", slog.Any("error", err))
		return nil, nil, err
	}

	otel.SetMeterProvider(meterProvider)
	slog.InfoContext(ctx, "OpenTelemetry meter provider set up and registered globally.")
	return meterProvider, meterProvider.Shutdown, nil
}

// initializeLoggerProvider sets up the OpenTelemetry logger provider and configures slog.
func initializeLoggerProvider(ctx context.Context, res *resource.Resource) (*sdklog.LoggerProvider, shutdownFunc, error) {
	slog.DebugContext(ctx, "Attempting to initialize OpenTelemetry logger provider.")
	loggerProvider, err := newLoggerProvider(ctx, res) // This also calls slog.SetDefault
	if err != nil {
		slog.ErrorContext(ctx, "Failed to initialize logger provider", slog.Any("error", err))
		return nil, nil, err
	}

	otelglobal.SetLoggerProvider(loggerProvider)
	// Note: newLoggerProvider calls slog.SetDefault, so subsequent slog messages go via OTel.
	// This log message will be processed by the OTel pipeline.
	slog.InfoContext(ctx, "OpenTelemetry logger provider configured for OTel SDK and slog global registration completed.")
	return loggerProvider, loggerProvider.Shutdown, nil
}

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call the returned shutdown function for proper cleanup.
func setupOTelSDK(ctx context.Context, cfg configura.Config) (shutdownFunc, error) {
	err := cfg.ConfigurationKeysRegistered(
		OTEL_ENABLED,
		OTEL_LOGS_ENABLED,
		OTEL_METRICS_ENABLED,
		OTEL_TRACES_ENABLED,
		OTEL_SERVICE_NAME,
	)
	if err != nil {
		slog.ErrorContext(ctx, "OpenTelemetry configuration keys missing", slog.Any("error", err))
		return nil, err
	}

	if !cfg.Bool(OTEL_ENABLED) {
		slog.InfoContext(ctx, "OpenTelemetry is disabled via OTEL_ENABLED. Skipping SDK setup.")
		return func(shutdownCtx context.Context) error {
			slog.DebugContext(shutdownCtx, "OpenTelemetry was disabled; no-op shutdown called.")
			return nil
		}, nil
	}

	slog.InfoContext(ctx, "OpenTelemetry is enabled. Proceeding with SDK setup.")
	var shutdownFuncs []shutdownFunc
	var cumulativeErr error

	// Master shutdown function that calls all registered component shutdown functions.
	masterShutdown := func(shutdownCtx context.Context) error {
		var shutdownErr error
		// Execute shutdown functions in reverse order of their addition.
		for i := len(shutdownFuncs) - 1; i >= 0; i-- {
			fn := shutdownFuncs[i]
			if e := fn(shutdownCtx); e != nil {
				shutdownErr = errors.Join(shutdownErr, e)
				// Individual component shutdown errors are logged by the components themselves or their wrappers.
			}
		}
		shutdownFuncs = nil // Prevent multiple calls.
		if shutdownErr != nil {
			slog.ErrorContext(shutdownCtx, "OpenTelemetry shutdown completed with errors", slog.Any("error", shutdownErr))
		} else {
			slog.InfoContext(shutdownCtx, "OpenTelemetry shutdown completed successfully.")
		}
		return shutdownErr
	}

	// Helper to handle errors during component setup.
	// It appends the error to cumulativeErr and attempts to shut down already initialized components.
	handleComponentSetupError := func(componentErr error, componentName string) {
		slog.ErrorContext(ctx, "Failed to setup OpenTelemetry component", slog.String("component", componentName), slog.Any("error", componentErr))
		cumulativeErr = errors.Join(cumulativeErr, componentErr)

		// Attempt to clean up what has been initialized so far.
		// The masterShutdown function itself logs errors.
		if shutdownUpToNowErr := masterShutdown(ctx); shutdownUpToNowErr != nil {
			cumulativeErr = errors.Join(cumulativeErr, shutdownUpToNowErr)
		}
	}

	// 1. Initialize Resource
	res, err := initializeResource(ctx, cfg)
	if err != nil {
		// No components to shut down yet, just return the resource error.
		return nil, err
	}

	// 2. Initialize Propagator
	initializePropagator(ctx) // Does not return error or shutdown func.

	// 3. Initialize Tracer Provider (if enabled)
	if configura.Fallback(cfg.Bool(OTEL_TRACES_ENABLED), false) {
		_, tracerShutdown, tpErr := initializeTracerProvider(ctx, res)
		if tpErr != nil {
			handleComponentSetupError(tpErr, "TracerProvider")
			return masterShutdown, cumulativeErr
		}
		shutdownFuncs = append(shutdownFuncs, tracerShutdown)
	} else {
		slog.InfoContext(ctx, "OpenTelemetry tracing is disabled via OTEL_TRACES_ENABLED. Skipping tracer provider setup.")
	}

	// 4. Initialize Meter Provider (if enabled)
	if configura.Fallback(cfg.Bool(OTEL_METRICS_ENABLED), false) {
		_, meterShutdown, mpErr := initializeMeterProvider(ctx, res)
		if mpErr != nil {
			handleComponentSetupError(mpErr, "MeterProvider")
			return masterShutdown, cumulativeErr
		}
		shutdownFuncs = append(shutdownFuncs, meterShutdown)
	} else {
		slog.InfoContext(ctx, "OpenTelemetry metrics are disabled via OTEL_METRICS_ENABLED. Skipping meter provider setup.")
	}

	// 5. Initialize Logger Provider (if enabled)
	// If OTEL_LOGS_ENABLED is true, slog's default logger will be reconfigured.
	// Subsequent logs from setupOTelSDK itself will go through this OTel pipeline.
	if configura.Fallback(cfg.Bool(OTEL_LOGS_ENABLED), false) {
		_, loggerShutdown, lpErr := initializeLoggerProvider(ctx, res) // This will change slog.Default
		if lpErr != nil {
			handleComponentSetupError(lpErr, "LoggerProvider")
			return masterShutdown, cumulativeErr
		}
		shutdownFuncs = append(shutdownFuncs, loggerShutdown)
		// This message goes through the OTel pipeline.
		slog.InfoContext(ctx, "slog is now bridged to OpenTelemetry logging pipeline.")
	} else {
		slog.InfoContext(ctx, "OpenTelemetry logging is disabled via OTEL_LOGS_ENABLED. Skipping logger provider setup.")
	}

	if cumulativeErr != nil {
		slog.ErrorContext(ctx, "OpenTelemetry SDK setup failed due to one or more component errors.", slog.Any("error", cumulativeErr))
		return masterShutdown, cumulativeErr
	}

	slog.InfoContext(ctx, "OpenTelemetry SDK setup completed successfully.")
	return masterShutdown, nil
}

// newTracerProvider creates a new trace.TracerProvider.
// It's kept as an internal detail for creating the specific type of provider.
func newTracerProvider(ctx context.Context, res *resource.Resource) (*trace.TracerProvider, error) {
	slog.DebugContext(ctx, "Creating stdout trace exporter.")
	traceExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create stdout trace exporter", slog.Any("error", err))
		return nil, err
	}
	slog.InfoContext(ctx, "Stdout trace exporter created.")

	tp := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter, trace.WithBatchTimeout(time.Second)), // Default is 5s. Set to 1s for dev/demo.
		trace.WithResource(res),
	)
	slog.InfoContext(ctx, "Tracer provider created.")
	return tp, nil
}

// newMeterProvider creates a new metric.MeterProvider.
// It's kept as an internal detail for creating the specific type of provider.
func newMeterProvider(ctx context.Context, res *resource.Resource) (*metric.MeterProvider, error) {
	slog.DebugContext(ctx, "Creating stdout metric exporter.")
	metricExporter, err := stdoutmetric.New()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create stdout metric exporter", slog.Any("error", err))
		return nil, err
	}
	slog.InfoContext(ctx, "Stdout metric exporter created.")

	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(3*time.Second))), // Default is 1m. Set to 3s for dev/demo.
		metric.WithResource(res),
	)
	slog.InfoContext(ctx, "Meter provider created.")
	return mp, nil
}

// newLoggerProvider creates an OTel sdklog.LoggerProvider and configures the default slog logger
// to route its logs through this OTel pipeline.
// It's kept as an internal detail for creating the specific type of provider and setting up slog.
func newLoggerProvider(ctx context.Context, res *resource.Resource) (*sdklog.LoggerProvider, error) {
	slog.DebugContext(ctx, "Creating OTel stdout log exporter.")
	logExporter, err := stdoutlog.New() // This exporter is for OTel logs.
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create OTel stdout log exporter", slog.Any("error", err))
		return nil, err
	}
	slog.InfoContext(ctx, "OTel stdout log exporter created.")

	slog.DebugContext(ctx, "Creating OTel SDK LoggerProvider.")
	// This is the OTel LoggerProvider that the OTel SDK will use.
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)
	slog.InfoContext(ctx, "OTel SDK LoggerProvider created.")

	// Configure the default slog logger to use an otelslog.Handler.
	// This handler will take slog records and forward them to the OTel LoggerProvider (lp).
	// Effectively, application logs made via slog will now go through the OTel logging pipeline.
	slog.SetDefault(slog.New(otelslog.NewHandler("", otelslog.WithLoggerProvider(lp))))

	// Important: From this point on, slog.InfoContext, slog.DebugContext, etc., from anywhere in the application
	// (that uses the default slog logger) will route through the OTel pipeline.
	slog.InfoContext(ctx, "Default slog logger replaced. Application logs via slog will now be processed by the OTel logging pipeline.")
	return lp, nil
}
