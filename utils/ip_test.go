package utils

import (
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/ponrove/configura" // Import for configura.Config, though not used by the function under test
)

// TestInRange tests the unexported inRange function
func TestInRange(t *testing.T) {
	tests := []struct {
		name      string
		r         ipRange
		ipAddress net.IP
		expected  bool
	}{
		// IPv4 tests
		{
			name: "IPv4 within range",
			r: ipRange{
				start: net.ParseIP("10.0.0.0"),
				end:   net.ParseIP("10.0.0.255"),
			},
			ipAddress: net.ParseIP("10.0.0.5"),
			expected:  true,
		},
		{
			name: "IPv4 at start of range",
			r: ipRange{
				start: net.ParseIP("10.0.0.0"),
				end:   net.ParseIP("10.0.0.255"),
			},
			ipAddress: net.ParseIP("10.0.0.0"),
			expected:  true,
		},
		{
			name: "IPv4 at end of range",
			r: ipRange{
				start: net.ParseIP("10.0.0.0"),
				end:   net.ParseIP("10.0.0.255"),
			},
			ipAddress: net.ParseIP("10.0.0.255"),
			expected:  true,
		},
		{
			name: "IPv4 just before end of range",
			r: ipRange{
				start: net.ParseIP("10.0.0.0"),
				end:   net.ParseIP("10.0.0.255"),
			},
			ipAddress: net.ParseIP("10.0.0.254"),
			expected:  true,
		},
		{
			name: "IPv4 before range",
			r: ipRange{
				start: net.ParseIP("10.0.0.0"),
				end:   net.ParseIP("10.0.0.255"),
			},
			ipAddress: net.ParseIP("9.255.255.255"),
			expected:  false,
		},
		{
			name: "IPv4 after range",
			r: ipRange{
				start: net.ParseIP("10.0.0.0"),
				end:   net.ParseIP("10.0.0.255"),
			},
			ipAddress: net.ParseIP("10.0.1.0"),
			expected:  false,
		},
		{
			name: "IPv4 single IP range - IP matches start/end",
			r: ipRange{
				start: net.ParseIP("192.168.1.1"),
				end:   net.ParseIP("192.168.1.1"),
			},
			ipAddress: net.ParseIP("192.168.1.1"),
			expected:  true,
		},

		// IPv6 tests
		{
			name: "IPv6 within range",
			r: ipRange{
				start: net.ParseIP("2001:db8::"),
				end:   net.ParseIP("2001:db8::ffff"),
			},
			ipAddress: net.ParseIP("2001:db8::1"),
			expected:  true,
		},
		{
			name: "IPv6 at start of range",
			r: ipRange{
				start: net.ParseIP("2001:db8::"),
				end:   net.ParseIP("2001:db8::ffff"),
			},
			ipAddress: net.ParseIP("2001:db8::"),
			expected:  true,
		},
		{
			name: "IPv6 at end of range",
			r: ipRange{
				start: net.ParseIP("2001:db8::"),
				end:   net.ParseIP("2001:db8::ffff"),
			},
			ipAddress: net.ParseIP("2001:db8::ffff"),
			expected:  true,
		},
		{
			name: "IPv6 just before end of range",
			r: ipRange{
				start: net.ParseIP("2001:db8::"),
				end:   net.ParseIP("2001:db8::ffff"),
			},
			ipAddress: net.ParseIP("2001:db8::fffe"),
			expected:  true,
		},
		{
			name: "IPv6 single IP range - IP matches start/end",
			r: ipRange{
				start: net.ParseIP("::1"),
				end:   net.ParseIP("::1"),
			},
			ipAddress: net.ParseIP("::1"),
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inRange(tt.r, tt.ipAddress); got != tt.expected {
				t.Errorf("inRange(%v, %s) = %v, want %v", tt.r, tt.ipAddress, got, tt.expected)
			}
		})
	}
}

func TestIsPrivateSubnet(t *testing.T) {
	tests := []struct {
		name      string
		ipAddress string
		expected  bool
	}{
		// IPv4 Private an Public
		{name: "IPv4 10.0.0.1 (Private)", ipAddress: "10.0.0.1", expected: true},
		{name: "IPv4 10.255.255.254 (Private - edge)", ipAddress: "10.255.255.254", expected: true},
		{name: "IPv4 10.255.255.255 (Private - end of range, exclusive)", ipAddress: "10.255.255.255", expected: true},
		{name: "IPv4 172.16.0.1 (Private)", ipAddress: "172.16.0.1", expected: true},
		{name: "IPv4 172.31.255.254 (Private - edge)", ipAddress: "172.31.255.254", expected: true},
		{name: "IPv4 172.31.255.255 (Private - end of range)", ipAddress: "172.31.255.255", expected: true},
		{name: "IPv4 192.168.1.1 (Private)", ipAddress: "192.168.1.1", expected: true},
		{name: "IPv4 127.0.0.1 (Loopback - Private by range)", ipAddress: "127.0.0.1", expected: true},
		{name: "IPv4 0.0.0.1 (Private by range 0.0.0.0/8)", ipAddress: "0.0.0.1", expected: true},
		// Based on current inRange and range def for 255.255.255.255/32, it is not marked private
		{name: "IPv4 255.255.255.255 (Broadcast)", ipAddress: "255.255.255.255", expected: true},
		{name: "IPv4 8.8.8.8 (Public)", ipAddress: "8.8.8.8", expected: false},
		{name: "IPv4 1.1.1.1 (Public)", ipAddress: "1.1.1.1", expected: false},
		{name: "IPv4 169.254.1.1 (Link-Local - Private)", ipAddress: "169.254.1.1", expected: true},

		// IPv6 Private and Public
		{name: "IPv6 ::1 (Loopback '::1' to '::1' range)", ipAddress: "::1", expected: true},
		{name: "IPv6 :: (Unspecified '::' to '::' range)", ipAddress: "::", expected: true},
		{name: "IPv6 fc00::1 (ULA - Private)", ipAddress: "fc00::1", expected: true},
		{name: "IPv6 fdff:ffff:ffff:ffff:ffff:ffff:ffff:fffe (ULA - Private Edge)", ipAddress: "fdff:ffff:ffff:ffff:ffff:ffff:ffff:fffe", expected: true},
		{name: "IPv6 fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff (ULA - Private End)", ipAddress: "fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", expected: true},
		{name: "IPv6 fe80::1 (Link-Local - Private)", ipAddress: "fe80::1", expected: true},
		{name: "IPv6 2001:db8::1 (Documentation - Public by typical standards, check ranges)", ipAddress: "2001:db8::1", expected: true}, // Covered by 2001:db8::/32
		{name: "IPv6 2001:4860:4860::8888 (Google DNS - Public)", ipAddress: "2001:4860:4860::8888", expected: false},

		// IPv4-mapped IPv6 addresses
		{name: "IPv4-mapped 10.0.0.1 (Private)", ipAddress: "::ffff:10.0.0.1", expected: true}, // Will be To4()
		{name: "IPv4-mapped 8.8.8.8 (Public)", ipAddress: "::ffff:8.8.8.8", expected: false},   // Will be To4()
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ipAddress)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ipAddress)
			}
			if got := IsPrivateSubnet(ip); got != tt.expected {
				t.Errorf("IsPrivateSubnet(%s) = %v, want %v", tt.ipAddress, got, tt.expected)
			}
		})
	}
}

func TestIPAddressFromRequest(t *testing.T) {
	// Helper to create a new request
	newRequest := func(method, url string, headers http.Header, remoteAddr string) *http.Request {
		req, _ := http.NewRequest(method, url, nil)
		if headers != nil {
			req.Header = headers
		}
		req.RemoteAddr = remoteAddr
		return req
	}

	// Mock configura.Config (not used by the function, so nil is fine)
	var mockCfg configura.Config = nil

	tests := []struct {
		name           string
		checkHeaders   []string
		requestHeaders http.Header
		remoteAddr     string
		expectedIP     string
	}{
		// Basic RemoteAddr cases
		{
			name:       "No headers, public RemoteAddr",
			remoteAddr: "8.8.8.8:12345",
			expectedIP: "8.8.8.8",
		},
		{
			name:       "No headers, private RemoteAddr",
			remoteAddr: "192.168.1.1:12345",
			expectedIP: "",
		},
		{
			name:       "No headers, loopback RemoteAddr",
			remoteAddr: "127.0.0.1:12345",
			expectedIP: "", // IsPrivateSubnet("127.0.0.1") is true, IsGlobalUnicast is false
		},
		{
			name:       "No headers, public IPv6 RemoteAddr",
			remoteAddr: "[2001:4860:4860::8888]:12345",
			expectedIP: "2001:4860:4860::8888",
		},
		{
			name:       "No headers, private IPv6 RemoteAddr (ULA)",
			remoteAddr: "[fc00::1]:12345",
			expectedIP: "", // IsPrivateSubnet("fc00::1") is true
		},
		{
			name:       "No headers, loopback IPv6 RemoteAddr",
			remoteAddr: "[::1]:12345",
			// IsPrivateSubnet("::1") is false by current ranges, IsGlobalUnicast is false
			// So `false && !false` is false. This means it depends on the check order.
			// `realIP.IsGlobalUnicast() && !IsPrivateSubnet(realIP)`
			// `IsGlobalUnicast(::1)` is false. So the condition is false.
			expectedIP: "",
		},

		// X-Forwarded-For tests
		{
			name:           "X-Forwarded-For: single public IP",
			requestHeaders: http.Header{"X-Forwarded-For": {"8.8.8.8"}},
			remoteAddr:     "192.168.1.1:12345",
			expectedIP:     "8.8.8.8",
		},
		{
			name:           "X-Forwarded-For: single private IP",
			requestHeaders: http.Header{"X-Forwarded-For": {"10.0.0.1"}},
			remoteAddr:     "8.8.8.8:12345", // Fallback to public RemoteAddr
			expectedIP:     "8.8.8.8",
		},
		{
			name:           "X-Forwarded-For: public, private",
			requestHeaders: http.Header{"X-Forwarded-For": {"8.8.8.8, 10.0.0.1"}}, // iterates right to left
			remoteAddr:     "192.168.1.1:12345",
			expectedIP:     "8.8.8.8", // 10.0.0.1 is private, 8.8.8.8 is public
		},
		{
			name:           "X-Forwarded-For: private, public",
			requestHeaders: http.Header{"X-Forwarded-For": {"10.0.0.1, 8.8.8.8"}},
			remoteAddr:     "192.168.1.1:12345",
			expectedIP:     "8.8.8.8",
		},
		{
			name:           "X-Forwarded-For: multiple public, chooses rightmost public",
			requestHeaders: http.Header{"X-Forwarded-For": {"1.1.1.1, 8.8.8.8, 10.0.0.1"}},
			remoteAddr:     "192.168.1.1:12345",
			expectedIP:     "8.8.8.8",
		},
		{
			name:           "X-Forwarded-For: with spaces",
			requestHeaders: http.Header{"X-Forwarded-For": {" 1.1.1.1 , 8.8.8.8 , 10.0.0.1 "}},
			remoteAddr:     "192.168.1.1:12345",
			expectedIP:     "8.8.8.8",
		},
		{
			name:           "X-Forwarded-For: all private, fallback to public RemoteAddr",
			requestHeaders: http.Header{"X-Forwarded-For": {"10.0.0.1, 192.168.0.5"}},
			remoteAddr:     "8.8.8.8:12345",
			expectedIP:     "8.8.8.8",
		},
		{
			name:           "X-Forwarded-For: all private, private RemoteAddr",
			requestHeaders: http.Header{"X-Forwarded-For": {"10.0.0.1, 192.168.0.5"}},
			remoteAddr:     "172.16.0.10:12345",
			expectedIP:     "",
		},

		// X-Real-Ip tests
		{
			name:           "X-Real-Ip: public IP",
			requestHeaders: http.Header{"X-Real-Ip": {"8.8.8.8"}},
			remoteAddr:     "192.168.1.1:12345",
			expectedIP:     "8.8.8.8",
		},
		{
			name:           "X-Real-Ip: private IP, fallback to public RemoteAddr",
			requestHeaders: http.Header{"X-Real-Ip": {"10.0.0.1"}},
			remoteAddr:     "8.8.8.8:12345",
			expectedIP:     "8.8.8.8",
		},

		// Header order (default: X-Forwarded-For before X-Real-Ip)
		{
			name: "Default header order: X-Forwarded-For (public) preferred over X-Real-Ip (public)",
			// checkHeaders is nil, so defaultHeaders is used
			requestHeaders: http.Header{
				"X-Forwarded-For": {"8.8.8.8"},
				"X-Real-Ip":       {"1.1.1.1"},
			},
			remoteAddr: "192.168.1.1:12345",
			expectedIP: "8.8.8.8",
		},
		{
			name: "Default header order: X-Forwarded-For (private), X-Real-Ip (public)",
			requestHeaders: http.Header{
				"X-Forwarded-For": {"10.0.0.1"},
				"X-Real-Ip":       {"1.1.1.1"},
			},
			remoteAddr: "192.168.1.1:12345",
			expectedIP: "1.1.1.1",
		},

		// Custom headers
		{
			name:           "Custom headers: My-Client-Ip (public)",
			checkHeaders:   []string{"My-Client-Ip"},
			requestHeaders: http.Header{"My-Client-Ip": {"8.8.8.8"}},
			remoteAddr:     "192.168.1.1:12345",
			expectedIP:     "8.8.8.8",
		},
		{
			name:           "Custom headers: X-Forwarded-For not in custom list, X-Real-Ip is",
			checkHeaders:   []string{"X-Real-Ip"},
			requestHeaders: http.Header{"X-Forwarded-For": {"8.8.8.8"}, "X-Real-Ip": {"1.1.1.1"}},
			remoteAddr:     "192.168.1.1:12345",
			expectedIP:     "1.1.1.1",
		},

		// Malformed/Invalid cases
		{
			name:       "Malformed RemoteAddr (no port)",
			remoteAddr: "8.8.8.8", // net.SplitHostPort will error
			expectedIP: "",
		},
		{
			name:       "Malformed RemoteAddr (invalid IP with port)",
			remoteAddr: "not-an-ip:12345",
			expectedIP: "", // net.ParseIP will return nil, then IsGlobalUnicast would panic if not checked
		},
		{
			name:           "X-Forwarded-For: invalid IP string",
			requestHeaders: http.Header{"X-Forwarded-For": {"not-an-ip"}},
			remoteAddr:     "8.8.8.8:12345",
			expectedIP:     "8.8.8.8", // Skips "not-an-ip" and falls back. If realIP was not checked for nil, it would panic.
		},
		{
			name:           "X-Forwarded-For: invalid IP mixed with valid",
			requestHeaders: http.Header{"X-Forwarded-For": {"not-an-ip, 8.8.8.8"}}, // processes right to left
			remoteAddr:     "192.168.1.1:12345",
			expectedIP:     "8.8.8.8", // Skips "not-an-ip"
		},
		{
			name:           "X-Forwarded-For: valid IP mixed with invalid",
			requestHeaders: http.Header{"X-Forwarded-For": {"8.8.8.8, not-an-ip"}},
			remoteAddr:     "192.168.1.1:12345",
			expectedIP:     "8.8.8.8", // This would be expected if panic is fixed and "not-an-ip" is skipped
		},
		{
			name:           "Empty X-Forwarded-For, public RemoteAddr",
			requestHeaders: http.Header{"X-Forwarded-For": {""}},
			remoteAddr:     "8.8.8.8:12345",
			expectedIP:     "8.8.8.8", // Expected if panic fixed
		},
		{
			name:           "X-Forwarded-For with only comma, public RemoteAddr",
			requestHeaders: http.Header{"X-Forwarded-For": {","}}, // splits into two empty strings
			remoteAddr:     "8.8.8.8:12345",
			expectedIP:     "8.8.8.8", // Expected if panic fixed
		},
		{
			name:       "RemoteAddr is just a port",
			remoteAddr: ":12345", // SplitHostPort gives empty ip
			expectedIP: "",       // net.ParseIP("") is nil
		},
		{
			name:           "very last range",
			requestHeaders: http.Header{"X-Forwarded-For": {"255.255.255.255"}},
			expectedIP:     "",
		},
		{
			name:           "very last range of private ip",
			requestHeaders: http.Header{"X-Forwarded-For": {"239.255.255.255"}},
			expectedIP:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r != nil {
					t.Errorf("IPAddressFromRequest panicked unexpectedly: %v", r)
				}
			}()

			req := newRequest("GET", "/", tt.requestHeaders, tt.remoteAddr)
			got := IPAddressFromRequest(mockCfg, tt.checkHeaders, req)

			if got != tt.expectedIP {
				var headers []string
				if tt.requestHeaders != nil {
					for k, v := range tt.requestHeaders {
						headers = append(headers, k+": "+strings.Join(v, ","))
					}
				}
				t.Errorf("IPAddressFromRequest with headers %v, remoteAddr %s = %q, want %q", headers, tt.remoteAddr, got, tt.expectedIP)
			}
		})
	}
}
