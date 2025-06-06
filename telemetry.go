package ponrunner

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/ponrove/configura"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
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
	OTEL_ENABLED                        configura.Variable[bool]   = "OTEL_ENABLED"
	OTEL_LOGS_ENABLED                   configura.Variable[bool]   = "OTEL_LOGS_ENABLED"
	OTEL_METRICS_ENABLED                configura.Variable[bool]   = "OTEL_METRICS_ENABLED"
	OTEL_TRACES_ENABLED                 configura.Variable[bool]   = "OTEL_TRACES_ENABLED"
	OTEL_SERVICE_NAME                   configura.Variable[string] = "OTEL_SERVICE_NAME"
	OTEL_EXPORTER_ENABLED               configura.Variable[bool]   = "OTEL_EXPORTER_ENABLED"
	OTEL_EXPORTER_OTLP_ENDPOINT         configura.Variable[string] = "OTEL_EXPORTER_OTLP_ENDPOINT"
	OTEL_EXPORTER_OTLP_TRACES_ENDPOINT  configura.Variable[string] = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
	OTEL_EXPORTER_OTLP_METRICS_ENDPOINT configura.Variable[string] = "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"
	OTEL_EXPORTER_OTLP_LOGS_ENDPOINT    configura.Variable[string] = "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"
	OTEL_EXPORTER_OTLP_HEADERS          configura.Variable[string] = "OTEL_EXPORTER_OTLP_HEADERS"
	OTEL_EXPORTER_OTLP_TRACES_HEADERS   configura.Variable[string] = "OTEL_EXPORTER_OTLP_TRACES_HEADERS"
	OTEL_EXPORTER_OTLP_METRICS_HEADERS  configura.Variable[string] = "OTEL_EXPORTER_OTLP_METRICS_HEADERS"
	OTEL_EXPORTER_OTLP_LOGS_HEADERS     configura.Variable[string] = "OTEL_EXPORTER_OTLP_LOGS_HEADERS"
	OTEL_EXPORTER_OTLP_TIMEOUT          configura.Variable[string] = "OTEL_EXPORTER_OTLP_TIMEOUT"
	OTEL_EXPORTER_OTLP_TRACES_TIMEOUT   configura.Variable[string] = "OTEL_EXPORTER_OTLP_TRACES_TIMEOUT"
	OTEL_EXPORTER_OTLP_METRICS_TIMEOUT  configura.Variable[string] = "OTEL_EXPORTER_OTLP_METRICS_TIMEOUT"
	OTEL_EXPORTER_OTLP_LOGS_TIMEOUT     configura.Variable[string] = "OTEL_EXPORTER_OTLP_LOGS_TIMEOUT"
	OTEL_EXPORTER_OTLP_PROTOCOL         configura.Variable[string] = "OTEL_EXPORTER_OTLP_PROTOCOL"
	OTEL_EXPORTER_OTLP_TRACES_PROTOCOL  configura.Variable[string] = "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"
	OTEL_EXPORTER_OTLP_METRICS_PROTOCOL configura.Variable[string] = "OTEL_EXPORTER_OTLP_METRICS_PROTOCOL"
	OTEL_EXPORTER_OTLP_LOGS_PROTOCOL    configura.Variable[string] = "OTEL_EXPORTER_OTLP_LOGS_PROTOCOL"
)

// Helper function to parse header strings (e.g., "key1=value1,key2=value2")
func parseHeaders(headerStr string) map[string]string {
	headers := make(map[string]string)
	if headerStr == "" {
		return headers
	}
	pairs := strings.Split(headerStr, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			if key != "" {
				headers[key] = value
			}
		}
	}
	return headers
}

// Helper function to parse duration strings (e.g., "5s")
func parseDuration(durationStr string, defaultDuration time.Duration) time.Duration {
	if durationStr == "" {
		return defaultDuration
	}
	d, err := time.ParseDuration(durationStr)
	if err != nil {
		slog.Warn("Failed to parse duration string, using default", slog.String("duration_string", durationStr), slog.Any("error", err), slog.Duration("default_duration", defaultDuration))
		return defaultDuration
	}
	return d
}

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
func initializeTracerProvider(ctx context.Context, res *resource.Resource, cfg configura.Config) (*trace.TracerProvider, shutdownFunc, error) {
	slog.DebugContext(ctx, "Attempting to initialize OpenTelemetry tracer provider.")
	tracerProvider, err := newTracerProvider(ctx, res, cfg)
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
func initializeMeterProvider(ctx context.Context, res *resource.Resource, cfg configura.Config) (*metric.MeterProvider, shutdownFunc, error) {
	slog.DebugContext(ctx, "Attempting to initialize OpenTelemetry meter provider.")
	meterProvider, err := newMeterProvider(ctx, res, cfg)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to initialize meter provider", slog.Any("error", err))
		return nil, nil, err
	}

	otel.SetMeterProvider(meterProvider)
	slog.InfoContext(ctx, "OpenTelemetry meter provider set up and registered globally.")
	return meterProvider, meterProvider.Shutdown, nil
}

// initializeLoggerProvider sets up the OpenTelemetry logger provider and configures slog.
func initializeLoggerProvider(ctx context.Context, res *resource.Resource, cfg configura.Config) (*sdklog.LoggerProvider, shutdownFunc, error) {
	slog.DebugContext(ctx, "Attempting to initialize OpenTelemetry logger provider.")
	loggerProvider, err := newLoggerProvider(ctx, res, cfg) // This also calls slog.SetDefault
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
		OTEL_EXPORTER_ENABLED,
		OTEL_EXPORTER_OTLP_ENDPOINT,
		OTEL_EXPORTER_OTLP_TRACES_ENDPOINT,
		OTEL_EXPORTER_OTLP_METRICS_ENDPOINT,
		OTEL_EXPORTER_OTLP_LOGS_ENDPOINT,
		OTEL_EXPORTER_OTLP_HEADERS,
		OTEL_EXPORTER_OTLP_TRACES_HEADERS,
		OTEL_EXPORTER_OTLP_METRICS_HEADERS,
		OTEL_EXPORTER_OTLP_LOGS_HEADERS,
		OTEL_EXPORTER_OTLP_TIMEOUT,
		OTEL_EXPORTER_OTLP_TRACES_TIMEOUT,
		OTEL_EXPORTER_OTLP_METRICS_TIMEOUT,
		OTEL_EXPORTER_OTLP_LOGS_TIMEOUT,
		OTEL_EXPORTER_OTLP_PROTOCOL,
		OTEL_EXPORTER_OTLP_TRACES_PROTOCOL,
		OTEL_EXPORTER_OTLP_METRICS_PROTOCOL,
		OTEL_EXPORTER_OTLP_LOGS_PROTOCOL,
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
		_, tracerShutdown, tpErr := initializeTracerProvider(ctx, res, cfg)
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
		_, meterShutdown, mpErr := initializeMeterProvider(ctx, res, cfg)
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
		_, loggerShutdown, lpErr := initializeLoggerProvider(ctx, res, cfg) // This will change slog.Default
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
func newTracerProvider(ctx context.Context, res *resource.Resource, cfg configura.Config) (*trace.TracerProvider, error) {
	var spanExporter trace.SpanExporter
	var err error

	if cfg.Bool(OTEL_EXPORTER_ENABLED) {
		slog.DebugContext(ctx, "OTLP exporter configured for traces. Attempting to create OTLP trace exporter.")
		protocol := strings.ToLower(configura.Fallback(cfg.String(OTEL_EXPORTER_OTLP_TRACES_PROTOCOL), cfg.String(OTEL_EXPORTER_OTLP_PROTOCOL)))
		endpoint := configura.Fallback(cfg.String(OTEL_EXPORTER_OTLP_TRACES_ENDPOINT), cfg.String(OTEL_EXPORTER_OTLP_ENDPOINT))

		if endpoint == "" {
			slog.WarnContext(ctx, "OTLP exporter is enabled but no endpoint is configured for traces. Falling back to stdout trace exporter.")
		} else {
			headers := parseHeaders(configura.Fallback(cfg.String(OTEL_EXPORTER_OTLP_TRACES_HEADERS), cfg.String(OTEL_EXPORTER_OTLP_HEADERS)))
			timeout := parseDuration(configura.Fallback(cfg.String(OTEL_EXPORTER_OTLP_TRACES_TIMEOUT), cfg.String(OTEL_EXPORTER_OTLP_TIMEOUT)), 10*time.Second)

			slog.InfoContext(ctx, "Configuring OTLP trace exporter.",
				slog.String("protocol", protocol),
				slog.String("endpoint", endpoint),
				slog.Duration("timeout", timeout),
				slog.Int("header_count", len(headers)))

			switch protocol {
			case "http", "http/protobuf":
				opts := []otlptracehttp.Option{
					otlptracehttp.WithEndpointURL(endpoint),
					otlptracehttp.WithTimeout(timeout),
				}
				if len(headers) > 0 {
					opts = append(opts, otlptracehttp.WithHeaders(headers))
				}
				if !strings.Contains(endpoint, "https://") {
					opts = append(opts, otlptracehttp.WithInsecure())
				}
				spanExporter, err = otlptracehttp.New(ctx, opts...)
			case "grpc":
				opts := []otlptracegrpc.Option{
					otlptracegrpc.WithEndpoint(endpoint),
					otlptracegrpc.WithTimeout(timeout),
				}
				if len(headers) > 0 {
					opts = append(opts, otlptracegrpc.WithHeaders(headers))
				}
				if !strings.Contains(endpoint, "https://") { // Assuming non-https endpoint implies insecure for gRPC too.
					opts = append(opts, otlptracegrpc.WithInsecure())
				}
				spanExporter, err = otlptracegrpc.New(ctx, opts...)
			default:
				return nil, errors.New("unsupported OTLP protocol for traces: " + protocol)
			}

			if err != nil {
				slog.ErrorContext(ctx, "Failed to create OTLP trace exporter, falling back to stdout.", slog.Any("error", err), slog.String("protocol", protocol))
				spanExporter = nil // Ensure it's nil so stdout is used
			} else {
				slog.InfoContext(ctx, "OTLP trace exporter created successfully.", slog.String("protocol", protocol), slog.String("endpoint", endpoint))
			}
		}
	}

	if spanExporter == nil { // Fallback to stdout if OTLP not enabled, not configured, or failed
		slog.DebugContext(ctx, "Creating stdout trace exporter as fallback or default.")
		spanExporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			slog.ErrorContext(ctx, "Failed to create stdout trace exporter", slog.Any("error", err))
			return nil, err
		}
		slog.InfoContext(ctx, "Stdout trace exporter created.")
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(spanExporter, trace.WithBatchTimeout(time.Second)), // Default is 5s. Set to 1s for dev/demo.
		trace.WithResource(res),
	)
	slog.InfoContext(ctx, "Tracer provider created.")
	return tp, err
}

// newMeterProvider creates a new metric.MeterProvider.
// It's kept as an internal detail for creating the specific type of provider.
func newMeterProvider(ctx context.Context, res *resource.Resource, cfg configura.Config) (*metric.MeterProvider, error) {
	var metricExporter metric.Exporter
	var err error

	if configura.Fallback(cfg.Bool(OTEL_EXPORTER_ENABLED), false) {
		slog.DebugContext(ctx, "OTLP exporter configured for metrics. Attempting to create OTLP metric exporter.")
		protocol := strings.ToLower(configura.Fallback(cfg.String(OTEL_EXPORTER_OTLP_METRICS_PROTOCOL), cfg.String(OTEL_EXPORTER_OTLP_PROTOCOL)))
		endpoint := configura.Fallback(cfg.String(OTEL_EXPORTER_OTLP_METRICS_ENDPOINT), cfg.String(OTEL_EXPORTER_OTLP_ENDPOINT))

		if endpoint == "" {
			slog.WarnContext(ctx, "OTLP exporter is enabled but no endpoint is configured for metrics. Falling back to stdout metric exporter.")
		} else {
			headers := parseHeaders(configura.Fallback(cfg.String(OTEL_EXPORTER_OTLP_METRICS_HEADERS), cfg.String(OTEL_EXPORTER_OTLP_HEADERS)))
			timeout := parseDuration(configura.Fallback(cfg.String(OTEL_EXPORTER_OTLP_METRICS_TIMEOUT), cfg.String(OTEL_EXPORTER_OTLP_TIMEOUT)), 10*time.Second)

			slog.InfoContext(ctx, "Configuring OTLP metric exporter.",
				slog.String("protocol", protocol),
				slog.String("endpoint", endpoint),
				slog.Duration("timeout", timeout),
				slog.Int("header_count", len(headers)))

			switch protocol {
			case "http", "http/protobuf":
				opts := []otlpmetrichttp.Option{
					otlpmetrichttp.WithEndpointURL(endpoint),
					otlpmetrichttp.WithTimeout(timeout),
				}
				if len(headers) > 0 {
					opts = append(opts, otlpmetrichttp.WithHeaders(headers))
				}
				if !strings.Contains(endpoint, "https://") {
					opts = append(opts, otlpmetrichttp.WithInsecure())
				}
				metricExporter, err = otlpmetrichttp.New(ctx, opts...)
			case "grpc":
				opts := []otlpmetricgrpc.Option{
					otlpmetricgrpc.WithEndpoint(endpoint),
					otlpmetricgrpc.WithTimeout(timeout),
				}
				if len(headers) > 0 {
					opts = append(opts, otlpmetricgrpc.WithHeaders(headers))
				}
				if !strings.Contains(endpoint, "https://") {
					opts = append(opts, otlpmetricgrpc.WithInsecure())
				}
				metricExporter, err = otlpmetricgrpc.New(ctx, opts...)
			default:
				err = errors.New("unsupported OTLP protocol for metrics: " + protocol)
			}

			if err != nil {
				slog.ErrorContext(ctx, "Failed to create OTLP metric exporter, falling back to stdout.", slog.Any("error", err), slog.String("protocol", protocol))
				metricExporter = nil // Ensure fallback
			} else {
				slog.InfoContext(ctx, "OTLP metric exporter created successfully.", slog.String("protocol", protocol), slog.String("endpoint", endpoint))
			}
		}
	}

	if metricExporter == nil { // Fallback to stdout if OTLP not enabled, not configured, or failed
		slog.DebugContext(ctx, "Creating stdout metric exporter as fallback or default.")
		metricExporter, err = stdoutmetric.New()
		if err != nil {
			slog.ErrorContext(ctx, "Failed to create stdout metric exporter", slog.Any("error", err))
			return nil, err
		}
		slog.InfoContext(ctx, "Stdout metric exporter created.")
	}

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
func newLoggerProvider(ctx context.Context, res *resource.Resource, cfg configura.Config) (*sdklog.LoggerProvider, error) {
	var logExporter sdklog.Exporter
	var err error

	if configura.Fallback(cfg.Bool(OTEL_EXPORTER_ENABLED), false) {
		slog.DebugContext(ctx, "OTLP exporter configured for logs. Attempting to create OTLP log exporter.")
		protocol := strings.ToLower(configura.Fallback(cfg.String(OTEL_EXPORTER_OTLP_LOGS_PROTOCOL), cfg.String(OTEL_EXPORTER_OTLP_PROTOCOL)))
		endpoint := configura.Fallback(cfg.String(OTEL_EXPORTER_OTLP_LOGS_ENDPOINT), cfg.String(OTEL_EXPORTER_OTLP_ENDPOINT))

		if endpoint == "" {
			slog.WarnContext(ctx, "OTLP exporter is enabled but no endpoint is configured for logs. Falling back to stdout log exporter.")
		} else {
			headers := parseHeaders(configura.Fallback(cfg.String(OTEL_EXPORTER_OTLP_LOGS_HEADERS), cfg.String(OTEL_EXPORTER_OTLP_HEADERS)))
			timeout := parseDuration(configura.Fallback(cfg.String(OTEL_EXPORTER_OTLP_LOGS_TIMEOUT), cfg.String(OTEL_EXPORTER_OTLP_TIMEOUT)), 10*time.Second)

			slog.InfoContext(ctx, "Configuring OTLP log exporter.",
				slog.String("protocol", protocol),
				slog.String("endpoint", endpoint),
				slog.Duration("timeout", timeout),
				slog.Int("header_count", len(headers)))

			switch protocol {
			case "http", "http/protobuf":
				opts := []otlploghttp.Option{
					otlploghttp.WithEndpointURL(endpoint),
					otlploghttp.WithTimeout(timeout),
				}
				if len(headers) > 0 {
					opts = append(opts, otlploghttp.WithHeaders(headers))
				}
				if !strings.Contains(endpoint, "https://") {
					opts = append(opts, otlploghttp.WithInsecure())
				}
				logExporter, err = otlploghttp.New(ctx, opts...)
			case "grpc":
				opts := []otlploggrpc.Option{
					otlploggrpc.WithEndpoint(endpoint),
					otlploggrpc.WithTimeout(timeout),
				}
				if len(headers) > 0 {
					opts = append(opts, otlploggrpc.WithHeaders(headers))
				}
				if !strings.Contains(endpoint, "https://") {
					opts = append(opts, otlploggrpc.WithInsecure())
				}
				logExporter, err = otlploggrpc.New(ctx, opts...)
			default:
				err = errors.New("unsupported OTLP protocol for logs: " + protocol)
			}

			if err != nil {
				slog.ErrorContext(ctx, "Failed to create OTLP log exporter, falling back to stdout.", slog.Any("error", err), slog.String("protocol", protocol))
				logExporter = nil // Ensure fallback
			} else {
				slog.InfoContext(ctx, "OTLP log exporter created successfully.", slog.String("protocol", protocol), slog.String("endpoint", endpoint))
			}
		}
	}

	if logExporter == nil { // Fallback to stdout if OTLP not enabled, not configured, or failed
		slog.DebugContext(ctx, "Creating OTel stdout log exporter as fallback or default.")
		logExporter, err = stdoutlog.New() // This exporter is for OTel logs.
		if err != nil {
			slog.ErrorContext(ctx, "Failed to create OTel stdout log exporter", slog.Any("error", err))
			return nil, err
		}
		slog.InfoContext(ctx, "OTel stdout log exporter created.")
	}

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
