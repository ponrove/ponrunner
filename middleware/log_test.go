package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/ponrove/configura"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	slogctx "github.com/veqryn/slog-context"
)

func defaultLogRequestConfig() configura.Config {
	cfg := configura.NewConfigImpl()
	cfg.RegString = map[configura.Variable[string]]string{
		REQUEST_LOG_FIELD_DURATION:       "duration",
		REQUEST_LOG_FIELD_REQUEST_METHOD: "requestMethod",
		REQUEST_LOG_FIELD_REQUEST_URL:    "requestURL",
		REQUEST_LOG_FIELD_USER_AGENT:     "userAgent",
		REQUEST_LOG_FIELD_REQUEST_SIZE:   "requestSize",
		REQUEST_LOG_FIELD_REMOTE_IP:      "remoteIP",
		REQUEST_LOG_FIELD_REFERER:        "referer",
		REQUEST_LOG_FIELD_PROTOCOL:       "protocol",
		REQUEST_LOG_FIELD_REQUEST_ID:     "requestID",
		REQUEST_LOG_FIELD_REAL_IP:        "realIP", // Ensure this is part of the default config
	}
	return cfg
}

// logOutput defines the structure for unmarshaling slog's JSON output.
// JSON tags must match the keys used in LogRequest, which are derived from
// defaultLogRequestConfig or hardcoded (like "statusCode", "responseSize").
type logOutput struct {
	Time          string `json:"time"`           // Standard slog field
	Level         string `json:"level"`          // Standard slog field
	Msg           string `json:"msg"`            // Standard slog field
	Duration      int64  `json:"duration"`       // Configured key (nanoseconds)
	RequestMethod string `json:"requestMethod"`  // Configured key
	RequestURL    string `json:"requestURL"`     // Configured key
	StatusCode    int    `json:"statusCode"`     // Hardcoded key in LogRequest
	ResponseSize  int    `json:"responseSize"`   // Hardcoded key in LogRequest
	UserAgent     string `json:"userAgent"`      // Configured key
	RequestSize   string `json:"requestSize"`    // Configured key
	RemoteIP      string `json:"remoteIP"`       // Configured key
	Referer       string `json:"referer"`        // Configured key
	Protocol      string `json:"protocol"`       // Configured key
	RequestID     string `json:"requestID"`      // Configured key
	RealIP        string `json:"realIP"`         // Configured key
}

// Helper to get path from a full URL string for matching log messages
func getPathFromURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		if strings.Contains(rawURL, "://") {
			return ""
		}
		return rawURL
	}
	return parsedURL.Path
}

func TestLogRequest(t *testing.T) {
	tests := []struct {
		name              string
		requestMethod     string
		requestURL        string
		requestBody       string
		userAgent         string
		remoteAddr        string
		referer           string
		protocol          string
		handlerStatusCode int
		handlerBody       string
		addRequestID      bool
		customLogFields   map[string]string
	}{
		{
			name:              "Simple GET request",
			requestMethod:     http.MethodGet,
			requestURL:        "/test/path?query=1",
			userAgent:         "test-agent/1.0",
			remoteAddr:        "192.0.2.1:12345",
			referer:           "http://example.com",
			protocol:          "HTTP/1.1",
			handlerStatusCode: http.StatusOK,
			handlerBody:       "Hello, world!",
			addRequestID:      true,
		},
		{
			name:              "POST request with body",
			requestMethod:     http.MethodPost,
			requestURL:        "/submit",
			requestBody:       "some data",
			userAgent:         "test-agent/2.0",
			remoteAddr:        "198.51.100.2:54321",
			protocol:          "HTTP/2.0",
			handlerStatusCode: http.StatusCreated,
			handlerBody:       `{"id": 123}`,
			addRequestID:      true,
		},
		{
			name:              "Request with no explicit status code write (defaults to 200)",
			requestMethod:     http.MethodGet,
			requestURL:        "/implicit-ok",
			handlerStatusCode: 0, // Will cause handler to not call WriteHeader explicitly
			handlerBody:       "Implicit OK",
			addRequestID:      false,
		},
		{
			name:              "Request with custom fields added to logger in handler",
			requestMethod:     http.MethodGet,
			requestURL:        "/custom-log",
			handlerStatusCode: http.StatusOK,
			handlerBody:       "custom log data",
			addRequestID:      true,
			customLogFields:   map[string]string{"custom_field": "custom_value", "user_id": "123"},
		},
		{
			name:              "Request resulting in 500",
			requestMethod:     http.MethodGet,
			requestURL:        "/error",
			handlerStatusCode: http.StatusInternalServerError,
			handlerBody:       "Internal Server Error",
			addRequestID:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			// Create a slog logger that writes JSON to logBuffer
			// Use a handler that doesn't add source by default to simplify output, unless needed.
			handlerOpts := slog.HandlerOptions{Level: slog.LevelInfo}
			testLogger := slog.New(slog.NewJSONHandler(&logBuffer, &handlerOpts))

			// The LogRequest middleware uses slog.Default() internally when initializing context logger.
			// So, we set our testLogger as the default for this test case.
			originalDefaultLogger := slog.Default()
			slog.SetDefault(testLogger)
			t.Cleanup(func() {
				slog.SetDefault(originalDefaultLogger)
			})

			handlerCalled := false
			mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true

				if tc.customLogFields != nil {
					// Retrieve the logger pointer from context (set by LogRequest middleware)
					currentSlogLoggerPtr := slogctx.FromCtx(r.Context()) // Returns *slog.Logger
					slogArgs := make([]any, 0, len(tc.customLogFields)*2)
					for k, v := range tc.customLogFields {
						slogArgs = append(slogArgs, k, v)
					}
					// Update the logger instance that the context's pointer points to
					*currentSlogLoggerPtr = *currentSlogLoggerPtr.With(slogArgs...)
				}

				if tc.handlerStatusCode != 0 {
					w.WriteHeader(tc.handlerStatusCode)
				}
				_, err := w.Write([]byte(tc.handlerBody))
				require.NoError(t, err)
			})

			req := httptest.NewRequest(tc.requestMethod, tc.requestURL, strings.NewReader(tc.requestBody))
			req.Header.Set("User-Agent", tc.userAgent)
			req.Header.Set("Referer", tc.referer)
			req.RemoteAddr = tc.remoteAddr
			req.Proto = tc.protocol

			if tc.requestBody != "" {
				req.Header.Set("Content-Length", strconv.Itoa(len(tc.requestBody)))
			} else {
				// For empty body, ContentLength should be 0 for r.ContentLength to be correct
				req.Header.Set("Content-Length", "0")
			}

			ctx := req.Context() // Start with the request's original context
			var expectedRequestID string
			if tc.addRequestID {
				expectedRequestID = "test-request-id"
				ctx = context.WithValue(ctx, middleware.RequestIDKey, expectedRequestID)
			}
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			config := defaultLogRequestConfig()
			middlewareToTest := LogRequest(config)(mockHandler)
			middlewareToTest.ServeHTTP(rr, req)

			// 1. Check if the handler was called
			assert.True(t, handlerCalled, "Handler was not called")

			// 2. Check response status code and body
			actualStatusCode := rr.Code
			expectedHandlerStatusCode := tc.handlerStatusCode
			if tc.handlerStatusCode == 0 && tc.handlerBody != "" {
				expectedHandlerStatusCode = http.StatusOK
			} else if tc.handlerStatusCode == 0 && tc.handlerBody == "" {
				expectedHandlerStatusCode = http.StatusOK
			}

			assert.Equal(t, expectedHandlerStatusCode, actualStatusCode, "Response status code mismatch")
			assert.Equal(t, tc.handlerBody, rr.Body.String(), "Response body mismatch")

			// 3. Check log output
			logContent := logBuffer.String()
			require.NotEmpty(t, logContent, "Log output should not be empty")

			// t.Logf("Raw log output: %s", logContent) // For debugging

			var loggedData logOutput
			err := json.Unmarshal([]byte(logContent), &loggedData)
			require.NoError(t, err, "Failed to unmarshal log output: %s", logContent)

			assert.Equal(t, "INFO", loggedData.Level) // slog uses uppercase "INFO"
			assert.NotEmpty(t, loggedData.Time, "Timestamp should be present")
			// Duration is in nanoseconds for slog
			assert.GreaterOrEqual(t, loggedData.Duration, int64(0), "Duration should be non-negative")
			assert.Less(t, loggedData.Duration, int64(1*time.Second), "Duration sanity check: < 1s (logged in ns)")

			assert.Equal(t, tc.requestMethod, loggedData.RequestMethod)
			assert.Equal(t, tc.requestURL, loggedData.RequestURL) // Full URL with query
			assert.Equal(t, expectedHandlerStatusCode, loggedData.StatusCode)
			assert.Equal(t, len(tc.handlerBody), loggedData.ResponseSize)

			assert.Equal(t, tc.userAgent, loggedData.UserAgent)

			expectedRequestSize := "0"
			if tc.requestBody != "" {
				expectedRequestSize = strconv.Itoa(len(tc.requestBody))
			}
			assert.Equal(t, expectedRequestSize, loggedData.RequestSize)

			assert.Equal(t, tc.remoteAddr, loggedData.RemoteIP) // This is r.RemoteAddr
			assert.Equal(t, tc.referer, loggedData.Referer)
			assert.Equal(t, tc.protocol, loggedData.Protocol)

			// The logged message is "HTTP request processed: METHOD /path"
			expectedLogMessage := fmt.Sprintf("HTTP request processed: %s %s", tc.requestMethod, getPathFromURL(tc.requestURL))
			assert.Equal(t, expectedLogMessage, loggedData.Msg)

			// Assuming GetIPAddressFromContext (from log.go) returns "" if no specific headers (X-Real-IP, X-Forwarded-For) are set
			assert.Equal(t, "", loggedData.RealIP, "RealIP should be empty as not set in test request headers")

			if tc.addRequestID {
				assert.Equal(t, expectedRequestID, loggedData.RequestID)
			} else {
				// middleware.GetReqID returns "" if no ID is in context.
				assert.Empty(t, loggedData.RequestID, "RequestID should be empty when not added by test")
			}

			// Check for custom fields
			if tc.customLogFields != nil {
				var fullLogMap map[string]any
				err = json.Unmarshal([]byte(logContent), &fullLogMap)
				require.NoError(t, err, "Failed to unmarshal full log output for custom fields: %s", logContent)
				for k, v := range tc.customLogFields {
					assert.Equal(t, v, fullLogMap[k], "Custom log field '%s' mismatch", k)
				}
			}
		})
	}
}

func TestCaptureResponseWriter_WriteHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	crw := &captureResponseWriter{ResponseWriter: rr}

	crw.WriteHeader(http.StatusAccepted)
	assert.Equal(t, http.StatusAccepted, crw.statusCode, "StatusCode mismatch after WriteHeader")
	assert.Equal(t, http.StatusAccepted, rr.Code, "Underlying ResponseWriter status code mismatch")
}

func TestCaptureResponseWriter_Write(t *testing.T) {
	rr := httptest.NewRecorder()
	crw := &captureResponseWriter{ResponseWriter: rr}

	testBody := []byte("hello")
	n, err := crw.Write(testBody)

	require.NoError(t, err)
	assert.Equal(t, len(testBody), n, "Write byte count mismatch")
	assert.Equal(t, len(testBody), crw.size, "Size mismatch after Write")
	assert.Equal(t, http.StatusOK, crw.statusCode, "StatusCode should default to 200 after Write if not set")
	assert.Equal(t, http.StatusOK, rr.Code, "Underlying ResponseWriter status code should be 200")
	assert.Equal(t, string(testBody), rr.Body.String(), "Body content mismatch")

	// Test writing again
	testBody2 := []byte(" world")
	n2, err2 := crw.Write(testBody2)
	require.NoError(t, err2)
	assert.Equal(t, len(testBody2), n2)
	assert.Equal(t, len(testBody)+len(testBody2), crw.size, "Size mismatch after second Write")
	assert.Equal(t, string(testBody)+string(testBody2), rr.Body.String(), "Body content mismatch after second Write")
}

func TestCaptureResponseWriter_Write_PreWriteHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	crw := &captureResponseWriter{ResponseWriter: rr}

	crw.WriteHeader(http.StatusCreated)
	testBody := []byte("created content")
	n, err := crw.Write(testBody)

	require.NoError(t, err)
	assert.Equal(t, len(testBody), n)
	assert.Equal(t, len(testBody), crw.size)
	assert.Equal(t, http.StatusCreated, crw.statusCode, "StatusCode should remain as set by WriteHeader")
	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.Equal(t, string(testBody), rr.Body.String())
}