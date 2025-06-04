package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/ponrove/configura"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	}
	return cfg
}

type logOutput struct {
	Level         string  `json:"level"`
	Duration      float64 `json:"duration"` // zerolog logs duration in ms by default
	RequestMethod string  `json:"requestMethod"`
	RequestURL    string  `json:"requestURL"`
	UserAgent     string  `json:"userAgent"`
	RequestSize   string  `json:"requestSize"`
	RemoteIP      string  `json:"remoteIP"`
	Referer       string  `json:"referer"`
	Protocol      string  `json:"protocol"`
	RequestID     string  `json:"requestID"`
	Message       string  `json:"message"`
}

func TestLogRequest(t *testing.T) {
	tests := []struct {
		name               string
		requestMethod      string
		requestURL         string
		requestBody        string
		userAgent          string
		remoteAddr         string
		referer            string
		protocol           string
		handlerStatusCode  int
		handlerBody        string
		expectedLogMessage string
		addRequestID       bool
		customLogFields    map[string]string
	}{
		{
			name:               "Simple GET request",
			requestMethod:      http.MethodGet,
			requestURL:         "/test/path?query=1",
			userAgent:          "test-agent/1.0",
			remoteAddr:         "192.0.2.1:12345",
			referer:            "http://example.com",
			protocol:           "HTTP/1.1",
			handlerStatusCode:  http.StatusOK,
			handlerBody:        "Hello, world!",
			expectedLogMessage: "[GET][200] /test/path",
			addRequestID:       true,
		},
		{
			name:               "POST request with body",
			requestMethod:      http.MethodPost,
			requestURL:         "/submit",
			requestBody:        "some data",
			userAgent:          "test-agent/2.0",
			remoteAddr:         "198.51.100.2:54321",
			protocol:           "HTTP/2.0",
			handlerStatusCode:  http.StatusCreated,
			handlerBody:        `{"id": 123}`,
			expectedLogMessage: "[POST][201] /submit",
			addRequestID:       true,
		},
		{
			name:               "Request with no explicit status code write (defaults to 200)",
			requestMethod:      http.MethodGet,
			requestURL:         "/implicit-ok",
			handlerStatusCode:  0, // Will cause handler to not call WriteHeader explicitly
			handlerBody:        "Implicit OK",
			expectedLogMessage: "[GET][200] /implicit-ok",
			addRequestID:       false,
		},
		{
			name:               "Request with custom fields added to logger in handler",
			requestMethod:      http.MethodGet,
			requestURL:         "/custom-log",
			handlerStatusCode:  http.StatusOK,
			handlerBody:        "custom log data",
			expectedLogMessage: "[GET][200] /custom-log",
			addRequestID:       true,
			customLogFields:    map[string]string{"custom_field": "custom_value", "user_id": "123"},
		},
		{
			name:               "Request resulting in 500",
			requestMethod:      http.MethodGet,
			requestURL:         "/error",
			handlerStatusCode:  http.StatusInternalServerError,
			handlerBody:        "Internal Server Error",
			expectedLogMessage: "[GET][500] /error",
			addRequestID:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			logger := zerolog.New(&logBuffer).With().Timestamp().Logger()

			handlerCalled := false
			mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true

				// Add custom fields to logger if specified for the test case
				if tc.customLogFields != nil {
					currentLogger := zerolog.Ctx(r.Context())
					l := currentLogger.With()
					for k, v := range tc.customLogFields {
						l = l.Str(k, v)
					}
					*currentLogger = l.Logger() // Update the logger in context
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
			}

			// Add logger to request context
			ctx := logger.WithContext(req.Context())

			// Add chi's request ID to context if needed
			var expectedRequestID string
			if tc.addRequestID {
				expectedRequestID = "test-request-id"
				ctx = context.WithValue(ctx, middleware.RequestIDKey, expectedRequestID)
			}
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			middlewareToTest := LogRequest(defaultLogRequestConfig())(mockHandler)
			middlewareToTest.ServeHTTP(rr, req)

			// 1. Check if the handler was called
			assert.True(t, handlerCalled, "Handler was not called")

			// 2. Check response status code and body
			actualStatusCode := rr.Code
			// If handler didn't set a status code, but wrote data, it defaults to 200
			expectedHandlerStatusCode := tc.handlerStatusCode
			if tc.handlerStatusCode == 0 && tc.handlerBody != "" {
				expectedHandlerStatusCode = http.StatusOK
			} else if tc.handlerStatusCode == 0 && tc.handlerBody == "" {
				// If no status code and no body, it also defaults to 200
				// This case might be tricky if the recorder defaults to 200 even if WriteHeader isn't called.
				// For the captureResponseWriter, if Write is called before WriteHeader, statusCode becomes 200.
				// If neither is called, statusCode remains 0, but recorder.Code will be 200.
				// The LogRequest middleware *should* log the 200 that is eventually sent.
				expectedHandlerStatusCode = http.StatusOK
			}

			assert.Equal(t, expectedHandlerStatusCode, actualStatusCode, "Response status code mismatch")
			assert.Equal(t, tc.handlerBody, rr.Body.String(), "Response body mismatch")

			// 3. Check log output
			logContent := logBuffer.String()
			require.NotEmpty(t, logContent, "Log output should not be empty")

			var loggedData logOutput
			err := json.Unmarshal([]byte(logContent), &loggedData)
			require.NoError(t, err, "Failed to unmarshal log output: %s", logContent)

			assert.Equal(t, "info", loggedData.Level)
			assert.Greater(t, loggedData.Duration, 0.0)                                  // Duration should be positive
			assert.Less(t, loggedData.Duration, float64(1*time.Second/time.Millisecond)) // Sanity check: < 1s

			assert.Equal(t, tc.requestMethod, loggedData.RequestMethod)
			assert.Equal(t, tc.requestURL, loggedData.RequestURL)
			assert.Equal(t, tc.userAgent, loggedData.UserAgent)

			expectedRequestSize := "0"
			if tc.requestBody != "" {
				expectedRequestSize = strconv.Itoa(len(tc.requestBody))
			}
			assert.Equal(t, expectedRequestSize, loggedData.RequestSize)

			assert.Equal(t, tc.remoteAddr, loggedData.RemoteIP)
			assert.Equal(t, tc.referer, loggedData.Referer)
			assert.Equal(t, tc.protocol, loggedData.Protocol)
			assert.Equal(t, tc.expectedLogMessage, loggedData.Message)

			if tc.addRequestID {
				assert.Equal(t, expectedRequestID, loggedData.RequestID)
			} else {
				assert.Empty(t, loggedData.RequestID, "RequestID should be empty when not added")
			}

			// Check for custom fields
			if tc.customLogFields != nil {
				var fullLogMap map[string]any
				err = json.Unmarshal([]byte(logContent), &fullLogMap)
				require.NoError(t, err, "Failed to unmarshal full log output: %s", logContent)
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
