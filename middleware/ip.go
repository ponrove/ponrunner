package middleware

import (
	"context"
	"iter"
	"net/http"
	"strings"

	"github.com/ponrove/configura"
	"github.com/ponrove/ponrunner/utils"
)

const (
	HTTP_HEADER_REAL_IP_OVERRIDE configura.Variable[string] = "HTTP_HEADER_REAL_IP" // Header to check for real IP address
)

// ctxIPAddressKey is a context key for storing the IP address.
type ctxIPAddressKey struct{}

// IPAddress is a middleware that extracts the IP address from the request and stores it in the request context.
func IPAddress(cfg configura.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var checkHeaders []string
			overrideHeader := cfg.String(HTTP_HEADER_REAL_IP_OVERRIDE)
			if overrideHeader != "" {
				splitHeader := strings.SplitSeq(overrideHeader, ",")
				next, stop := iter.Pull(splitHeader)
				defer stop()
				for {
					header, ok := next()
					if !ok {
						break
					}
					checkHeaders = append(checkHeaders, strings.TrimSpace(header))
				}
			}

			ip := utils.IPAddressFromRequest(cfg, checkHeaders, r)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxIPAddressKey{}, ip)))
		})
	}
}

// GetIPAddressFromContext retrieves the IP address from the context.
func GetIPAddressFromContext(ctx context.Context) string {
	if ip, ok := ctx.Value(ctxIPAddressKey{}).(string); ok {
		return ip
	}
	return ""
}
