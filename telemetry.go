package ponrunner

import (
	"context"
	"errors"
	"time"

	"github.com/ponrove/configura"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	otelglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log" // Corrected import for SDK logger, aliased to sdklog
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
		return nil, err // Return early if configuration keys registration fails
	}

	// Check if OpenTelemetry is enabled
	if !configura.Fallback(cfg.Bool(OTEL_ENABLED), false) {
		log.Info().Msg("OpenTelemetry is disabled via OTEL_ENABLED. Skipping SDK setup.")
		// Return a no-op shutdown function and no error
		return func(context.Context) error {
			log.Debug().Msg("OpenTelemetry was disabled; no-op shutdown called.")
			return nil
		}, nil
	}

	log.Info().Msg("OpenTelemetry is enabled. Proceeding with SDK setup.")
	var shutdownFuncs []shutdownFunc
	var cumulativeErr error // This will store the cumulative error from setup steps

	// Define a resource for the application.
	res, err := resource.New(ctx,
		resource.WithAttributes(
			// Use OTEL_SERVICE_NAME from config, fallback to "ponrove" if not set
			semconv.ServiceName(configura.Fallback(cfg.String(OTEL_SERVICE_NAME), "ponrove")),
		),
	)
	if err != nil {
		log.Error().Err(err).Msg("failed to create OpenTelemetry resource")
		return nil, err // Early return if resource creation fails
	}

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown := func(ctx context.Context) error {
		var shutdownErr error
		// Execute shutdown functions in reverse order of their addition.
		for i := len(shutdownFuncs) - 1; i >= 0; i-- {
			fn := shutdownFuncs[i]
			if e := fn(ctx); e != nil {
				shutdownErr = errors.Join(shutdownErr, e)
				log.Error().Err(e).Msg("error during OpenTelemetry component shutdown")
			}
		}
		shutdownFuncs = nil // Clear the slice to prevent multiple calls if shutdown is called again.
		if shutdownErr != nil {
			log.Error().Err(shutdownErr).Msg("OpenTelemetry shutdown completed with errors")
		} else {
			log.Info().Msg("OpenTelemetry shutdown completed successfully")
		}
		return shutdownErr
	}

	// handleErr calls shutdown for cleanup and ensures that all errors are returned.
	// This local helper simplifies error handling during setup.
	handleErr := func(componentErr error, componentName string) {
		log.Error().Err(componentErr).Msgf("failed to setup OpenTelemetry %s", componentName)
		// Store the cumulative error in the outer 'cumulativeErr' variable.
		if cumulativeErr == nil { // if this is the first error
			cumulativeErr = componentErr
		} else {
			cumulativeErr = errors.Join(cumulativeErr, componentErr)
		}
		// Attempt to shutdown previously initialized components
		// The shutdown function itself logs errors, so we just join its potential error.
		if shutdownUpToNowErr := shutdown(ctx); shutdownUpToNowErr != nil {
			cumulativeErr = errors.Join(cumulativeErr, shutdownUpToNowErr)
		}
	}

	// Set up propagator.
	otel.SetTextMapPropagator(newPropagator())
	log.Info().Msg("OpenTelemetry propagator set up")

	if configura.Fallback(cfg.Bool(OTEL_TRACES_ENABLED), true) {
		// Set up trace provider.
		tracerProvider, tpErr := newTracerProvider(ctx, res)
		if tpErr != nil {
			handleErr(tpErr, "tracer provider")
			// Even if handleErr calls shutdown, we must return the shutdown func from setupOTelSDK
			// so the caller can potentially call it again if they wish (though it would be a no-op after the first successful call).
			return shutdown, cumulativeErr
		}
		shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
		otel.SetTracerProvider(tracerProvider)
		log.Info().Msg("OpenTelemetry tracer provider set up")
	} else {
		log.Info().Msg("OpenTelemetry tracing is disabled via OTEL_TRACES_ENABLED. Skipping tracer provider setup.")
	}

	if configura.Fallback(cfg.Bool(OTEL_METRICS_ENABLED), true) {
		// Set up meter provider.
		meterProvider, mpErr := newMeterProvider(ctx, res)
		if mpErr != nil {
			handleErr(mpErr, "meter provider")
			return shutdown, cumulativeErr
		}
		shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
		otel.SetMeterProvider(meterProvider)
		log.Info().Msg("OpenTelemetry meter provider set up")
	} else {
		log.Info().Msg("OpenTelemetry metrics are disabled via OTEL_METRICS_ENABLED. Skipping meter provider setup.")
	}

	if configura.Fallback(cfg.Bool(OTEL_LOGS_ENABLED), true) {
		// Set up logger provider.
		loggerProvider, lpErr := newLoggerProvider(ctx, res)
		if lpErr != nil {
			handleErr(lpErr, "logger provider")
			return shutdown, cumulativeErr
		}
		shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
		otelglobal.SetLoggerProvider(loggerProvider)
		log.Info().Msg("OpenTelemetry logger provider set up")

		if cumulativeErr != nil { // If any component failed to set up
			log.Error().Err(cumulativeErr).Msg("OpenTelemetry SDK setup failed due to one or more component errors")
			return shutdown, cumulativeErr // Return the cumulative error
		}
	} else {
		log.Info().Msg("OpenTelemetry logging is disabled via OTEL_LOGS_ENABLED. Skipping logger provider setup.")
	}

	log.Info().Msg("OpenTelemetry SDK setup completed successfully")
	return shutdown, nil // cumulativeErr will be nil if all setup steps succeeded
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTracerProvider(ctx context.Context, res *resource.Resource) (*trace.TracerProvider, error) {
	log.Debug().Msg("Initializing stdout trace exporter")
	traceExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		log.Error().Err(err).Msg("failed to create stdout trace exporter")
		return nil, err
	}
	log.Info().Msg("Stdout trace exporter initialized")

	tp := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter, trace.WithBatchTimeout(time.Second)), // Default is 5s. Set to 1s for dev/demo.
		trace.WithResource(res),
	)
	log.Info().Msg("Tracer provider created")
	return tp, nil
}

func newMeterProvider(ctx context.Context, res *resource.Resource) (*metric.MeterProvider, error) {
	log.Debug().Msg("Initializing stdout metric exporter")
	metricExporter, err := stdoutmetric.New()
	if err != nil {
		log.Error().Err(err).Msg("failed to create stdout metric exporter")
		return nil, err
	}
	log.Info().Msg("Stdout metric exporter initialized")

	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(3*time.Second))), // Default is 1m. Set to 3s for dev/demo.
		metric.WithResource(res),
	)
	log.Info().Msg("Meter provider created")
	return mp, nil
}

func newLoggerProvider(ctx context.Context, res *resource.Resource) (*sdklog.LoggerProvider, error) {
	log.Debug().Msg("Initializing stdout log exporter")
	logExporter, err := stdoutlog.New() // Default is os.Stdout
	if err != nil {
		log.Error().Err(err).Msg("failed to create stdout log exporter")
		return nil, err
	}
	log.Info().Msg("Stdout log exporter initialized")

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)), // Using BatchProcessor as per common examples
		sdklog.WithResource(res),
	)
	log.Info().Msg("Logger provider created")
	return lp, nil
}
