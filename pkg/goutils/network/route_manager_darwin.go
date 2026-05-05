//go:build darwin

package network

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
)

// List of commands to run to configure
// a tunnel interface and routes
//
// local network's gateway to the internet: 192.168.1.1
// local tunnel IP for LHS of tunnel: 192.168.111.194
// peer tunnel IP for RHS of tunnel which is also the tunnel's internet gateway: 192.168.111.1
// external IP of wireguard peer: 34.204.21.102
//
// * configure tunnel network interface
// 			/sbin/ifconfig utun6 inet 192.168.111.194/32 192.168.111.194 up
// * configure route to wireguard overlay network via tunnel network interface
// 			/sbin/route add -inet -net 192.168.111.1 -interface utun6
// * configure route to peer's public endpoint via network interface connected to the internet
// 			/sbin/route add inet -net 34.204.21.102 192.168.1.1 255.255.255.255
// * configure route to send all other traffic through the tunnel by create two routes splitting
//   the entire IPv4 range of 0.0.0.0/0. i.e. 0.0.0.0/1 and 128.0.0.0/1
// 			/sbin/route add inet -net 0.0.0.0 192.168.111.1 128.0.0.0
// 			/sbin/route add inet -net 128.0.0.0 192.168.111.1 128.0.0.0
//   OR see
//      https://itectec.com/askdifferent/macos-how-to-change-the-default-gateway-of-a-mac-osx-machine/
//
// * cleanup
// 			/sbin/route delete inet -net 34.204.21.102

type routeManager struct {	
	nc *networkContext
}

type routableInterface struct {
	ifaceName,
	gatewayAddress string
}

func (c *networkContext) NewRouteManager() (RouteManager, error) {

	rm := &routeManager{
		nc: c,
	}
	return rm, nil
}

func (m *routeManager) GetDefaultInterface() (RoutableInterface, error) {
	return &routableInterface{
		ifaceName:      Network.DefaultIPv4Route.InterfaceName,
		gatewayAddress: Network.DefaultIPv4Route.GatewayIP.String(),
	}, nil
}

func (m *routeManager) GetRoutableInterface(ifaceName string) (RoutableInterface, error) {
	for _, r := range Network.StaticRoutes {
		if r.InterfaceName == ifaceName {
			return &routableInterface{
				ifaceName:      r.InterfaceName,
				gatewayAddress: r.GatewayIP.String(),
			}, nil
		}
	}
	return &routableInterface{
		ifaceName: ifaceName,
	}, nil
}

func (m *routeManager) NewRoutableInterface(ifaceName, address string) (RoutableInterface, error) {

	var (
		err error

		ip    net.IP
		ipNet *net.IPNet
	)

	if ip, ipNet, err = net.ParseCIDR(address); err != nil {
		return nil, err
	}
	size, _ := ipNet.Mask.Size()
	if (size == 32) {
		// default to a /24 if address 
		// does not indicate network
		ipNet.Mask = net.CIDRMask(24, 32)
	}

	gatewayIP := ip.Mask(ipNet.Mask);
	IncIP(gatewayIP)
	gatewayAddress := gatewayIP.String()

	// add tunnel IP to local tunnel interface
	if err = ifconfig.Run([]string{ ifaceName, "inet", address, ip.String(), "up" }); err != nil {
		return nil, err
	}	
	// create route to tunnel gateway via tunnel interface
	if err = route.Run([]string{ "add", "-inet", "-net", gatewayAddress, "-interface", ifaceName }); err != nil {
		return nil, err
	}
	return &routableInterface{
		gatewayAddress: gatewayAddress,
	}, nil
}

func (m *routeManager) NewFilterRouter(denyAll bool) (FilterRouter, error) {
	return nil, fmt.Errorf("filter router has not been implemented for darwin os")
}

func (m *routeManager) AddExternalRouteToIPs(ips []string) error {

	var (
		err error

		ipCidr string
	)
	defaultGateway := Network.DefaultIPv4Route.GatewayIP.String()

	for _, ip := range ips {
		if strings.HasSuffix(ip, ".0") {
			ipCidr = ip[:len(ip)-2]+"/32"
		} else {
			ipCidr = ip+"/32"
		}
		if err = route.Run([]string{ "add", "-inet", "-net", ipCidr, defaultGateway, "255.255.255.255" }); err != nil {
			logger.ErrorMessage(
				"routeManager.AddExternalRouteToIPs(): Unable to add static route to IP %s via gateway %s: %s", 
				ip, defaultGateway, err.Error())
		} else {
			m.nc.routedIPs = append(m.nc.routedIPs, ipCidr)
		}
	}
	return nil
}

func (m *routeManager) AddDefaultRoute(gateway string) error {
	return addDefaultRoute(gateway)
}

func (m *routeManager) Clear() {
	
	var (
		err error
	)
	defaultGateway := Network.DefaultIPv4Route.GatewayIP.String()

	// clear routed ips if any
	if len(m.nc.routedIPs) > 0 {
		for _, ip := range m.nc.routedIPs {
			if err = route.Run([]string{ "delete", "-inet", "-net", ip }); err != nil {
				logger.ErrorMessage("routeManager.Clear(): Deleting route to %s: %s", ip, err.Error())
			}
		}
		m.nc.routedIPs = nil
	}

	// clear default route if any
	if err = addDefaultRoute(defaultGateway); err != nil {
		logger.ErrorMessage("routeManager.Clear(): Restoring default route to %s: %s", defaultGateway, err.Error())
	}
}

func (i *routableInterface) Name() string {
	return i.ifaceName
}

func (i *routableInterface) Address4() (string, string, error) {
	return "", "", nil
}

func (i *routableInterface) Address6() (string, string, error) {
	return "", "", nil
}

func (i *routableInterface) MakeDefaultRoute() error {
	return addDefaultRoute(i.gatewayAddress)
}

func addDefaultRoute(gateway string) error {

	var (
		err error
	)

	// create default route via interface's gateway
	if err = route.Run([]string{ "delete", "default" }); err != nil {
		return err
	}
	if err = route.Run([]string{ "add", "default", gateway }); err != nil {
		return err
	}
	return nil
}

func (i *routableInterface) SetSecurityGroups(sgs []SecurityGroup) error {
	return nil
}

func (i *routableInterface) DeleteSecurityGroups(sgs []SecurityGroup) error {
	return nil
}

func (i *routableInterface) ForwardPortTo(proto Protocol, dstPort int, forwardPort int, forwardIP netip.Addr) (string, error) {
	return "", nil
}

func (i *routableInterface) DeletePortForwardedTo(proto Protocol, dstPort int, forwardPort int, forwardIP netip.Addr) error {
	return nil
}

func (i *routableInterface) FowardTrafficTo(dstItf RoutableInterface, srcNetwork, dstNetwork string, withNat bool) (string, error) {
	// NAT packets from src to network this itf is connected
	// https://gist.github.com/ozel/93c48ff291b83ac648278f0562167b7e
	// https://apple.stackexchange.com/questions/363099/how-to-forward-traffic-from-one-machine-to-another-with-pfctl
	return "", nil
}

func (i *routableInterface) DeleteTrafficForwardedTo(dstItf RoutableInterface, srcNetwork, dstNetwork string) error {
	// NAT packets from src to network this itf is connected
	// https://gist.github.com/ozel/93c48ff291b83ac648278f0562167b7e
	// https://apple.stackexchange.com/questions/363099/how-to-forward-traffic-from-one-machine-to-another-with-pfctl
	return nil
}

func (i *routableInterface) FowardTrafficFrom(srcItf RoutableInterface, srcNetwork, dstNetwork string, withNat bool) (string, error) {
	// NAT packets from src to network this itf is connected
	// https://gist.github.com/ozel/93c48ff291b83ac648278f0562167b7e
	// https://apple.stackexchange.com/questions/363099/how-to-forward-traffic-from-one-machine-to-another-with-pfctl
	return "", nil
}

func (i *routableInterface) DeleteTrafficForwardedFrom(srcItf RoutableInterface, srcNetwork, destNetwork string) error {
	// NAT packets from src to network this itf is connected
	// https://gist.github.com/ozel/93c48ff291b83ac648278f0562167b7e
	// https://apple.stackexchange.com/questions/363099/how-to-forward-traffic-from-one-machine-to-another-with-pfctl
	return nil
}
