package network

import (
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
)

var privateNetworks = []netip.Prefix{
	// Private or RFC 1918 address space
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	// Unique Local Addresses (RFC 4193)
	netip.MustParsePrefix("fc00::/7"),
	// Link-Local Addresses
	netip.MustParsePrefix("fe80::/10"),
}

// returns whether the given ipv4 or 
// ipv6 address is a private address
func IsPrivateAddr(addr netip.Addr) bool {
	for _, privateNetwork := range privateNetworks {
		if privateNetwork.Contains(addr) {
			return true
		}
	}
	return false
}

// resolves the given list of domain
// names and returns their corresponding
// as two lists. The first list will
// either be a flattened list of all
// resolved ips as stringsor just the 
// first resolved ip giving a 1:1 mapping 
// to the given names. The second list is 
// a list of ips resolved for each name.
func ResolveNames(dnsNames []string, flatten bool) ([]string, [][]string, error) {

	var (
		err error

		resolvedIPs []net.IP
	)
	ipsFlat  := []string{}
	namedIPs := [][]string{}

	for _, name := range dnsNames {	
		if resolvedIPs, err = net.LookupIP(name); err != nil {
			return nil, nil, err
		}
		ips := []string{}
		namedIPs = append(namedIPs, ips)

		for i, ip := range resolvedIPs {			
			ipAddr := ip.String()
			ips = append(ips, ipAddr)
			if flatten {
				ipsFlat = append(ipsFlat, ipAddr)
			} else if i == 0 {
				ipsFlat = append(ipsFlat, ipAddr)
			}
		}	
	}

	return ipsFlat, namedIPs, nil
}

// test tcp connection
func CanConnect(host string, port int) bool {

	endpoint := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", endpoint, time.Second)
	if err != nil {
		logger.TraceMessage(
			"Connectivity test to '%s' failed: %s",
			endpoint, err.Error(),
		)
		return false
	}

	defer conn.Close()
	return true
}
