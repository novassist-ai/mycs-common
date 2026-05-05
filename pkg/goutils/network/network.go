package network

import "net/netip"

const (
	WORLD4 = "0.0.0.0/0"
	WORLD6 = "::/0"
	LAN4   = "4"
	LAN6   = "6"
)

type Protocol string
const (
	ICMP Protocol = "icmp"
	TCP  Protocol = "tcp"
	UDP  Protocol = "udp"
)

type SecurityGroup struct {

	Deny bool // default to allow

	SrcNetwork,
	DstNetwork netip.Prefix

	Oifname string

	Ports []PortGroup
}
type PortGroup struct {
	Proto Protocol

	FromPort, 
	ToPort int
}

type NetworkContext interface {	
	DefaultDeviceName() string
	DefaultInterface() string
	DefaultGateway() string
	DefaultIP() string

	DisableIPv6() error

	NewDNSManager() (DNSManager, error)
	NewRouteManager() (RouteManager, error)

	Clear()
}

type DNSManager interface {
	AddDNSServers(servers []string) error
	AddSearchDomains(domains []string) error

	Clear()
}

type RouteManager interface {
	GetDefaultInterface() (RoutableInterface, error)
	GetRoutableInterface(ifaceName string) (RoutableInterface, error)
	NewRoutableInterface(ifaceName, tunAddress string) (RoutableInterface, error)

	NewFilterRouter(denyAll bool) (FilterRouter, error)

	AddExternalRouteToIPs(ips []string) error
	AddDefaultRoute(gateway string) error

	Clear()
}

type RoutableInterface interface {
	Name() string
	Address4() (string, string, error)
	Address6() (string, string, error)
	MakeDefaultRoute() error

	SetSecurityGroups(sgs []SecurityGroup) error
	DeleteSecurityGroups(sgs []SecurityGroup) error

	ForwardPortTo(proto Protocol, dstPort int, forwardPort int, forwardIP netip.Addr) (string, error)
	DeletePortForwardedTo(proto Protocol, dstPort int, forwardPort int, forwardIP netip.Addr) error

	FowardTrafficTo(dstItf RoutableInterface, srcNetwork, dstNetwork string, withNat bool) (string, error)
	DeleteTrafficForwardedTo(dstItf RoutableInterface, srcNetwork, dstNetwork string) error
	FowardTrafficFrom(srcItf RoutableInterface, srcNetwork, dstNetwork string, withNat bool) (string, error)
	DeleteTrafficForwardedFrom(srcItf RoutableInterface, srcNetwork, destNetwork string) error
}

type FilterRouter interface {

	AddIPsToDenyList(ips []netip.Addr) error
	DeleteIPsFromDenyList(ips []netip.Addr) error
	
	AddIPsToAllowList(ips []netip.Addr) error
	DeleteIPsFromAllowList(ips []netip.Addr) error

	SetSecurityGroups(sgs []SecurityGroup, iifName string) error
	DeleteSecurityGroups(sgs []SecurityGroup, iifName string) error

	ForwardPort(dstPort, forwardPort int, forwardIP netip.Addr, proto Protocol) (string, error)
	DeleteForwardPort(dstPort, forwardPort int, forwardIP netip.Addr, proto Protocol) error

	ForwardPortOnIP(dstPort, forwardPort int, dstIP, forwardIP netip.Addr, proto Protocol) (string, error)
	DeleteForwardPortOnIP(dstPort, forwardPort int, dstIP, forwardIP netip.Addr, proto Protocol) error

	ForwardTraffic(srcItfName, dstItfName string, srcNetwork, dstNetwork netip.Prefix, withNat bool) (string, error)
	DeleteForwardTraffic(srcItfName, dstItfName string, srcNetwork, dstNetwork netip.Prefix) error

	DeleteFilter(key string) error

	Clear()
}
