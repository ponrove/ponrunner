package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ponrove/configura"
	"github.com/ponrove/ponrunner/middleware"
)

func TestIPAddress_NoOverride(t *testing.T) {
	cfg := configura.NewConfigImpl()
	// HTTP_HEADER_REAL_IP_OVERRIDE is not set, cfg.String will return its default ("")

	ipMiddleware := middleware.IPAddress(cfg)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := middleware.GetIPAddressFromContext(r.Context())
		if ip != "155.234.211.2" {
			t.Errorf("Expected IP 155.234.211.2, got %s", ip)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "155.234.211.2:12345"
	rr := httptest.NewRecorder()
	ipMiddleware(testHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestIPAddress_SingleOverride_ValidPublicIP(t *testing.T) {
	cfg := configura.NewConfigImpl()
	configura.WriteConfiguration(cfg, map[configura.Variable[string]]string{middleware.HTTP_HEADER_REAL_IP_OVERRIDE: "X-Forwarded-For"})

	ipMiddleware := middleware.IPAddress(cfg)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := middleware.GetIPAddressFromContext(r.Context())
		if ip != "204.0.113.1" {
			t.Errorf("Expected IP 204.0.113.1, got %s", ip)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	req.Header.Set("X-Forwarded-For", "204.0.113.1, 198.51.100.2")
	rr := httptest.NewRecorder()
	ipMiddleware(testHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestIPAddress_SingleOverride_PrivateIPInHeader_FallbackToPublicRemoteAddr(t *testing.T) {
	cfg := configura.NewConfigImpl()
	configura.WriteConfiguration(cfg, map[configura.Variable[string]]string{middleware.HTTP_HEADER_REAL_IP_OVERRIDE: "X-Real-IP"})

	ipMiddleware := middleware.IPAddress(cfg)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := middleware.GetIPAddressFromContext(r.Context())
		if ip != "203.0.114.10" {
			t.Errorf("Expected IP 204.0.114.10, got %s", ip)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.114.10:12345"
	req.Header.Set("X-Real-IP", "10.0.0.1")
	rr := httptest.NewRecorder()
	ipMiddleware(testHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestIPAddress_SingleOverride_EmptyHeaderValue_FallbackToRemoteAddr(t *testing.T) {
	cfg := configura.NewConfigImpl()
	configura.WriteConfiguration(cfg, map[configura.Variable[string]]string{middleware.HTTP_HEADER_REAL_IP_OVERRIDE: "X-Client-IP"})

	ipMiddleware := middleware.IPAddress(cfg)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := middleware.GetIPAddressFromContext(r.Context())
		if ip != "" {
			t.Errorf("Expected IP no IP, got %s", ip)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	req.Header.Set("X-Client-IP", "")
	rr := httptest.NewRecorder()
	ipMiddleware(testHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestIPAddress_SingleOverride_HeaderNotPresent_FallbackToRemoteAddr(t *testing.T) {
	cfg := configura.NewConfigImpl()
	configura.WriteConfiguration(cfg, map[configura.Variable[string]]string{middleware.HTTP_HEADER_REAL_IP_OVERRIDE: "X-App-IP"})

	ipMiddleware := middleware.IPAddress(cfg)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := middleware.GetIPAddressFromContext(r.Context())
		if ip != "" {
			t.Errorf("Expected IP no IP, got %s", ip)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	rr := httptest.NewRecorder()
	ipMiddleware(testHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestIPAddress_MultipleOverrides_FirstHeaderValidPublic(t *testing.T) {
	cfg := configura.NewConfigImpl()
	configura.WriteConfiguration(cfg, map[configura.Variable[string]]string{middleware.HTTP_HEADER_REAL_IP_OVERRIDE: "X-True-Client-IP,X-Forwarded-For"})

	ipMiddleware := middleware.IPAddress(cfg)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := middleware.GetIPAddressFromContext(r.Context())
		if ip != "203.0.114.5" {
			t.Errorf("Expected IP 203.0.114.5, got %s", ip)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	req.Header.Set("X-True-Client-IP", "203.0.114.5")
	req.Header.Set("X-Forwarded-For", "198.51.100.2")
	rr := httptest.NewRecorder()
	ipMiddleware(testHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestIPAddress_MultipleOverrides_SubsequentHeaderValidPublic_WithSpacesInConfig(t *testing.T) {
	cfg := configura.NewConfigImpl()
	configura.WriteConfiguration(cfg, map[configura.Variable[string]]string{middleware.HTTP_HEADER_REAL_IP_OVERRIDE: "  X-NonExistent-IP , X-Real-IP  ,  CF-Connecting-IP "})

	ipMiddleware := middleware.IPAddress(cfg)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := middleware.GetIPAddressFromContext(r.Context())
		if ip != "203.0.114.25" {
			t.Errorf("Expected IP 203.0.114.25, got %s", ip)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	req.Header.Set("X-Real-IP", "10.0.0.5")            // Private, skipped
	req.Header.Set("CF-Connecting-IP", "203.0.114.25") // Public, picked
	rr := httptest.NewRecorder()
	ipMiddleware(testHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestIPAddress_MultipleOverrides_NonePublicInHeaders_FallbackToPublicRemoteAddr(t *testing.T) {
	cfg := configura.NewConfigImpl()
	configura.WriteConfiguration(cfg, map[configura.Variable[string]]string{middleware.HTTP_HEADER_REAL_IP_OVERRIDE: "X-Header1,X-Header2"})

	ipMiddleware := middleware.IPAddress(cfg)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := middleware.GetIPAddressFromContext(r.Context())
		if ip != "198.52.100.100" {
			t.Errorf("Expected IP 198.52.100.100, got %s", ip)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.52.100.100:12345"
	req.Header.Set("X-Header1", "10.0.0.1")
	req.Header.Set("X-Header2", "172.16.0.1,invalid")
	rr := httptest.NewRecorder()
	ipMiddleware(testHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestIPAddress_HeaderWithMultipleIPs_FirstPublicPicked(t *testing.T) {
	cfg := configura.NewConfigImpl()
	configura.WriteConfiguration(cfg, map[configura.Variable[string]]string{middleware.HTTP_HEADER_REAL_IP_OVERRIDE: "X-Forwarded-For"})

	ipMiddleware := middleware.IPAddress(cfg)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := middleware.GetIPAddressFromContext(r.Context())
		if ip != "203.0.114.50" {
			t.Errorf("Expected IP 203.0.114.50, got %s", ip)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 172.16.0.1, 203.0.114.50, 198.51.100.2")
	rr := httptest.NewRecorder()
	ipMiddleware(testHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestIPAddress_RemoteAddrOnly_Private(t *testing.T) {
	cfg := configura.NewConfigImpl()
	// No override header

	ipMiddleware := middleware.IPAddress(cfg)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := middleware.GetIPAddressFromContext(r.Context())
		if ip != "" { // utils.IPAddressFromRequest returns private if it's the only option
			t.Errorf("Expected IP no IP, got %s", ip)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345" // Private IP
	rr := httptest.NewRecorder()
	ipMiddleware(testHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestIPAddress_NoValidIPAvailable(t *testing.T) {
	cfg := configura.NewConfigImpl()
	configura.WriteConfiguration(cfg, map[configura.Variable[string]]string{middleware.HTTP_HEADER_REAL_IP_OVERRIDE: "X-NonExistent-Header,X-Invalid-IP-Hdr"})

	ipMiddleware := middleware.IPAddress(cfg)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := middleware.GetIPAddressFromContext(r.Context())
		if ip != "" {
			t.Errorf("Expected empty IP, got '%s'", ip)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "invalid-remote-addr" // Invalid RemoteAddr
	req.Header.Set("X-Invalid-IP-Hdr", "not an ip, also not an ip")
	rr := httptest.NewRecorder()
	ipMiddleware(testHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestGetIPAddressFromContext_NotFound(t *testing.T) {
	// This tests if GetIPAddressFromContext returns "" when the key is not in the context.
	// The IPAddress middleware tests implicitly cover the "found" case.
	ctx := context.Background() // An empty context will not have the IP address key.
	ip := middleware.GetIPAddressFromContext(ctx)
	if ip != "" {
		t.Errorf("Expected empty IP when key is not in context, got %s", ip)
	}
}

func TestIPAddress_IPv6(t *testing.T) {
	cfg := configura.NewConfigImpl()
	configura.WriteConfiguration(cfg, map[configura.Variable[string]]string{middleware.HTTP_HEADER_REAL_IP_OVERRIDE: "X-IPv6-Real-IP"})

	ipMiddleware := middleware.IPAddress(cfg)
	expectedIP := "2003:db8::1"

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := middleware.GetIPAddressFromContext(r.Context())
		if ip != expectedIP {
			t.Errorf("Expected IP %s, got %s", expectedIP, ip)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "[2001:db8::ffff]:12345" // Fallback IPv6
	req.Header.Set("X-IPv6-Real-IP", expectedIP)
	rr := httptest.NewRecorder()
	ipMiddleware(testHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestIPAddress_IPv6_LoopbackInHeader_FallbackToPublicRemoteIPv6(t *testing.T) {
	cfg := configura.NewConfigImpl()
	configura.WriteConfiguration(cfg, map[configura.Variable[string]]string{middleware.HTTP_HEADER_REAL_IP_OVERRIDE: "X-Forwarded-For"})

	ipMiddleware := middleware.IPAddress(cfg)
	expectedIP := "2003:db8:85a3::8a2e:370:7334"

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := middleware.GetIPAddressFromContext(r.Context())
		if ip != expectedIP {
			t.Errorf("Expected IP %s, got %s", expectedIP, ip)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "[" + expectedIP + "]:54321"     // Public IPv6
	req.Header.Set("X-Forwarded-For", "::1, fc00::1") // Loopback and ULA (private)
	rr := httptest.NewRecorder()
	ipMiddleware(testHandler).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}
