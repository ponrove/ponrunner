package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/ponrove/configura"
	"github.com/rs/zerolog"
)

// Custom response writer to capture the status code and response size, for logging.
type captureResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

// Ensure the captureResponseWriter implements the http.ResponseWriter interface at compile time.
var _ http.ResponseWriter = &captureResponseWriter{}

// Interceptor that captures the status code of the response.
func (crw *captureResponseWriter) WriteHeader(code int) {
	crw.statusCode = code
	crw.ResponseWriter.WriteHeader(code)
}

// Interceptor that writes the size of the response body to the captureResponseWriter.
func (crw *captureResponseWriter) Write(b []byte) (int, error) {
	if crw.statusCode == 0 {
		crw.statusCode = http.StatusOK
	}
	size, err := crw.ResponseWriter.Write(b)
	crw.size += size
	return size, err
}

const (
	REQUEST_LOG_FIELD_DURATION       configura.Variable[string] = "REQUEST_LOG_FIELD_DURATION"
	REQUEST_LOG_FIELD_REQUEST_METHOD configura.Variable[string] = "REQUEST_LOG_FIELD_REQUEST_METHOD"
	REQUEST_LOG_FIELD_REQUEST_URL    configura.Variable[string] = "REQUEST_LOG_FIELD_REQUEST_URL"
	REQUEST_LOG_FIELD_USER_AGENT     configura.Variable[string] = "REQUEST_LOG_FIELD_USER_AGENT"
	REQUEST_LOG_FIELD_REQUEST_SIZE   configura.Variable[string] = "REQUEST_LOG_FIELD_REQUEST_SIZE"
	REQUEST_LOG_FIELD_REMOTE_IP      configura.Variable[string] = "REQUEST_LOG_FIELD_REMOTE_IP"
	REQUEST_LOG_FIELD_REFERER        configura.Variable[string] = "REQUEST_LOG_FIELD_REFERER"
	REQUEST_LOG_FIELD_PROTOCOL       configura.Variable[string] = "REQUEST_LOG_FIELD_PROTOCOL"
	REQUEST_LOG_FIELD_REQUEST_ID     configura.Variable[string] = "REQUEST_LOG_FIELD_REQUEST_ID"
)

// fallback is a helper function that returns the fallback value if the provided value is empty.
func fallback(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// LogRequest is a middleware that logs the request details on each request.
func LogRequest(cfg configura.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Fetch a logger from context (or default logger), and make a child logger from it.
			childLogger := zerolog.Ctx(r.Context()).With().Logger()

			// Add the logger to the request context, to pass it downstream.
			r = r.WithContext(childLogger.WithContext(r.Context()))

			// Wrap the response writer to capture the status code and response size, on writes.
			crw := &captureResponseWriter{ResponseWriter: w}
			next.ServeHTTP(crw, r)

			// Fetch logger from context, potentially has new fields and contextual data added by the endpoints.
			postLogger := zerolog.Ctx(r.Context())
			postLogger.Info().
				Dur(fallback(cfg.String(REQUEST_LOG_FIELD_DURATION), "duration"), time.Since(start)).
				Str(fallback(cfg.String(REQUEST_LOG_FIELD_REQUEST_METHOD), "method"), r.Method).
				Str(fallback(cfg.String(REQUEST_LOG_FIELD_REQUEST_URL), "requestURL"), r.URL.String()).
				Str(fallback(cfg.String(REQUEST_LOG_FIELD_USER_AGENT), "userAgent"), r.Header.Get("User-Agent")).
				Str(fallback(cfg.String(REQUEST_LOG_FIELD_REQUEST_SIZE), "requestSize"), strconv.FormatInt(r.ContentLength, 10)).
				Str(fallback(cfg.String(REQUEST_LOG_FIELD_REMOTE_IP), "remoteIP"), r.RemoteAddr).
				Str(fallback(cfg.String(REQUEST_LOG_FIELD_REFERER), "referer"), r.Header.Get("Referer")).
				Str(fallback(cfg.String(REQUEST_LOG_FIELD_PROTOCOL), "protocol"), r.Proto).
				Str(fallback(cfg.String(REQUEST_LOG_FIELD_REQUEST_ID), "requestID"), middleware.GetReqID(r.Context())).
				Msgf("[%s][%d] %s", r.Method, crw.statusCode, r.URL.Path)
		})
	}
}
