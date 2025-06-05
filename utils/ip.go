package utils

import (
	"bytes"
	"net"
	"net/http"
	"strings"

	"github.com/ponrove/configura"
)

// ipRange - a structure that holds the start and end of a range of ip addresses
type ipRange struct {
	start net.IP
	end   net.IP
}

// inRange - check to see if a given ip address is within a range given
func inRange(r ipRange, ipAddress net.IP) bool {
	// strcmp type byte comparison
	if bytes.Compare(ipAddress, r.start) >= 0 && bytes.Compare(ipAddress, r.end) <= 0 {
		return true
	}
	return false
}

// privateRanges - a list of private ip ranges
var privateRangesV4 = []ipRange{
	{
		// 0.0.0.0/8
		start: net.ParseIP("0.0.0.0"),
		end:   net.ParseIP("0.255.255.255"),
	},
	{
		// 10.0.0.0/8
		start: net.ParseIP("10.0.0.0"),
		end:   net.ParseIP("10.255.255.255"),
	},
	{
		// 100.64.0.0/10
		start: net.ParseIP("100.64.0.0"),
		end:   net.ParseIP("100.127.255.255"),
	},
	{
		// 127.0.0.0/8
		start: net.ParseIP("127.0.0.0"),
		end:   net.ParseIP("127.255.255.255"),
	},
	{
		// 169.254.0.0/16
		start: net.ParseIP("169.254.0.0"),
		end:   net.ParseIP("169.254.255.255"),
	},
	{
		// 172.16.0.0/12
		start: net.ParseIP("172.16.0.0"),
		end:   net.ParseIP("172.31.255.255"),
	},
	{
		// 192.0.0.0/24
		start: net.ParseIP("192.0.0.0"),
		end:   net.ParseIP("192.0.0.255"),
	},
	{
		// 192.0.2.0/24
		start: net.ParseIP("192.0.2.0"),
		end:   net.ParseIP("192.0.2.255"),
	},
	{
		// 192.88.99.0/24
		start: net.ParseIP("192.88.99.0"),
		end:   net.ParseIP("192.88.99.255"),
	},
	{
		// 192.168.0.0/16
		start: net.ParseIP("192.168.0.0"),
		end:   net.ParseIP("192.168.255.255"),
	},
	{
		// 198.18.0.0/15
		start: net.ParseIP("198.18.0.0"),
		end:   net.ParseIP("198.19.255.255"),
	},
	{
		// 198.51.100.0/24
		start: net.ParseIP("198.51.100.0"),
		end:   net.ParseIP("198.51.100.255"),
	},
	{
		// 203.0.113.0/24
		start: net.ParseIP("203.0.113.0"),
		end:   net.ParseIP("203.0.113.255"),
	},
	{
		// 224.0.0.0/4
		start: net.ParseIP("224.0.0.0"),
		end:   net.ParseIP("239.255.255.255"),
	},
	{
		// 233.252.0.0/24
		start: net.ParseIP("233.252.0.0"),
		end:   net.ParseIP("233.252.0.255"),
	},
	{
		// 240.0.0.4/4
		start: net.ParseIP("240.0.0.0"),
		end:   net.ParseIP("255.255.255.254"),
	},
	{
		// 255.255.255.255/32
		start: net.ParseIP("255.255.255.255"),
		end:   net.ParseIP("255.255.255.255"),
	},
}

var privateRangesV6 = []ipRange{
	{
		// ::/128
		start: net.ParseIP("::"),
		end:   net.ParseIP("::"),
	},
	{
		// ::1/128
		start: net.ParseIP("::1"),
		end:   net.ParseIP("::1"),
	},
	{
		// ::ffff:0:0/96
		start: net.ParseIP("::ffff:0:0"),
		end:   net.ParseIP("::ffff:ffff:ffff"),
	},
	{
		// ::ffff:0:0:0/96
		start: net.ParseIP("::ffff:0:0:0"),
		end:   net.ParseIP("::ffff:0:ffff:ffff"),
	},
	{
		// 64:ff9b::/96
		start: net.ParseIP("64:ff9b::0:0"),
		end:   net.ParseIP("64:ff9b::ffff:ffff"),
	},
	{
		// 64:ff9b:1::/48
		start: net.ParseIP("64:ff9b:1::"),
		end:   net.ParseIP("64:ff9b:1:ffff:ffff:ffff:ffff:ffff"),
	},
	{
		// 100::/64
		start: net.ParseIP("100::"),
		end:   net.ParseIP("100::ffff:ffff:ffff:ffff"),
	},
	{
		// 2001::/32
		start: net.ParseIP("2001::"),
		end:   net.ParseIP("2001:0:ffff:ffff:ffff:ffff:ffff:ffff"),
	},
	{
		// 2001:20::/28
		start: net.ParseIP("2001:20::"),
		end:   net.ParseIP("2001:2f:ffff:ffff:ffff:ffff:ffff:ffff"),
	},
	{
		// 2001:db8::/32
		start: net.ParseIP("2001:db8::"),
		end:   net.ParseIP("2001:db8:ffff:ffff:ffff:ffff:ffff:ffff"),
	},
	{
		// 2002::/16
		start: net.ParseIP("2002::"),
		end:   net.ParseIP("2002:ffff:ffff:ffff:ffff:ffff:ffff:ffff"),
	},
	{
		// 3fff::/20
		start: net.ParseIP("3fff::"),
		end:   net.ParseIP("3fff:fff:ffff:ffff:ffff:ffff:ffff:ffff"),
	},
	{
		// 5f00::/16
		start: net.ParseIP("5f00::"),
		end:   net.ParseIP("5f00:ffff:ffff:ffff:ffff:ffff:ffff:ffff"),
	},
	{
		// fc00::/7
		start: net.ParseIP("fc00::"),
		end:   net.ParseIP("fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"),
	},
	{
		// fe80::/64 from fe80::/10
		start: net.ParseIP("fe80::"),
		end:   net.ParseIP("fe80::ffff:ffff:ffff:ffff"),
	},
	{
		// ff00::/8
		start: net.ParseIP("ff00::"),
		end:   net.ParseIP("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"),
	},
}

// IsPrivateSubnet - check to see if this ip is in a private subnet
func IsPrivateSubnet(ipAddress net.IP) bool {
	// my use case is only concerned with ipv4 atm
	if ipCheck := ipAddress.To4(); ipCheck != nil {
		// iterate over all our ranges
		for _, r := range privateRangesV4 {
			// check if this ip is in a private range
			if inRange(r, ipAddress) {
				// log.Debug().Str("ip", ipAddress.String()).Msgf("IP %s is in private range, stuck on index %d, %s - %s", ipAddress.String(), i, r.start.String(), r.end.String())
				return true
			}
		}
	} else {
		// iterate over all our ranges
		for _, r := range privateRangesV6 {
			// check if this ip is in a private range
			if inRange(r, ipAddress) {
				return true
			}
		}
	}

	return false
}

var defaultHeaders = []string{
	"X-Forwarded-For",
	"X-Real-Ip",
	"Http-Forwarded-For",
	"Http-Forwarded",
	"Http-X-Cluster-Client-Ip",
	"Http-X-Forwarded-For",
	"Http-X-Forwarded",
	"Http-Client-Ip",
}

// IPAddressFromRequest extracts the IP address from the request headers or remote address. Optionally checks specified
// headers for the IP address, falling back to the remote address if no valid public IP is found.
func IPAddressFromRequest(cfg configura.Config, checkHeaders []string, r *http.Request) string {
	if len(checkHeaders) == 0 {
		checkHeaders = append(checkHeaders, defaultHeaders...)
	}

	for _, h := range checkHeaders {
		addresses := strings.Split(r.Header.Get(h), ",")
		// march from right to left until we get a public address
		// that will be the address right before our proxy.
		for i := len(addresses) - 1; i >= 0; i-- {
			ip := strings.TrimSpace(addresses[i])
			// header can contain spaces too, strip those out.
			realIP := net.ParseIP(ip)
			if realIP == nil {
				// not a valid IP, go to next
				continue
			}
			if !realIP.IsGlobalUnicast() || IsPrivateSubnet(realIP) {
				// bad address, go to next
				continue
			}
			return ip
		}
	}

	// if no public address, use the remote address
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		realIP := net.ParseIP(ip)
		if realIP == nil {
			// not a valid IP, return empty
			return ""
		}

		if realIP.IsGlobalUnicast() && !IsPrivateSubnet(realIP) {
			return ip
		}
	}

	return ""
}
