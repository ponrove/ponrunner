package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/ponrove/configura"
	slogctx "github.com/veqryn/slog-context"
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
	REQUEST_LOG_FIELD_REAL_IP        configura.Variable[string] = "REQUEST_LOG_FIELD_REAL_IP"
	REQUEST_LOG_FIELD_STATUS_CODE    configura.Variable[string] = "REQUEST_LOG_FIELD_STATUS_CODE"
	REQUEST_LOG_FIELD_RESPONSE_SIZE  configura.Variable[string] = "REQUEST_LOG_FIELD_RESPONSE_SIZE"
	REQUEST_LOG_FIELD_HOST           configura.Variable[string] = "REQUEST_LOG_FIELD_HOST"
	REQUEST_LOG_FIELD_FINGERPRINT    configura.Variable[string] = "REQUEST_LOG_FIELD_FINGERPRINT"
)

// LogRequest is a middleware that logs the request details on each request.
func LogRequest(cfg configura.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Add the logger to the request context, to pass it downstream.
			r = r.WithContext(slogctx.NewCtx(r.Context(), slog.Default()))

			// Wrap the response writer to capture the status code and response size, on writes.
			crw := &captureResponseWriter{ResponseWriter: w}
			next.ServeHTTP(crw, r)

			// Fetch logger from context. It will include any attributes added by slogctx throughout the request.
			// If no logger is in context, it falls back to slog.Default().
			logger := slogctx.FromCtx(r.Context())

			logger.LogAttrs(r.Context(), slog.LevelInfo,
				fmt.Sprintf("HTTP request processed: %s %s", r.Method, r.URL.Path),
				slog.Duration(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_DURATION), "duration"), time.Since(start)),
				slog.String(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_REQUEST_METHOD), "method"), r.Method),
				slog.String(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_REQUEST_URL), "request_url"), r.URL.String()),
				slog.Int(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_STATUS_CODE), "status_code"), crw.statusCode),
				slog.Int(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_RESPONSE_SIZE), "response_size"), crw.size),
				slog.String(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_HOST), "host"), r.Header.Get("Host")),
				slog.String(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_FINGERPRINT), "fingerprint"), r.Header.Get("X-Fingerprint")),
				slog.String(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_USER_AGENT), "user_agent"), r.Header.Get("User-Agent")),
				slog.String(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_REQUEST_SIZE), "request_size"), strconv.FormatInt(r.ContentLength, 10)),
				slog.String(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_REMOTE_IP), "remote_ip"), r.RemoteAddr),
				slog.String(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_REFERER), "referer"), r.Header.Get("Referer")),
				slog.String(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_PROTOCOL), "protocol"), r.Proto),
				slog.String(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_REAL_IP), "real_ip"), GetIPAddressFromContext(r.Context())),
				slog.String(configura.Fallback(cfg.String(REQUEST_LOG_FIELD_REQUEST_ID), "request_id"), middleware.GetReqID(r.Context())),
			)
		})
	}
}
