//go:build linux

package network

import (
	"fmt"
	"net"
	"net/netip"

	"github.com/vishvananda/netlink"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
)

type routeManager struct {
	nc *networkContext
	pfr *packetFilterRouter
}

type routableInterface struct {
	link           netlink.Link
	gatewayAddress net.IP

	m *routeManager
}

func (c *networkContext) NewRouteManager() (RouteManager, error) {

	rm := &routeManager{
		nc:  c,
		pfr: &packetFilterRouter{},
	}
	c.rm = rm
	return c.rm, nil
}

func (m *routeManager) GetDefaultInterface() (RoutableInterface, error) {

	var (
		err error
	)
	itf := routableInterface{m:m}

	if Network.DefaultIPv4Route != nil {
		if itf.link, err = netlink.LinkByName(Network.DefaultIPv4Route.InterfaceName); err != nil {
			return nil, err
		}
		itf.gatewayAddress = Network.DefaultIPv4Route.GatewayIP.AsSlice()

	} else if Network.DefaultIPv6Route != nil {
		if itf.link, err = netlink.LinkByName(Network.DefaultIPv6Route.InterfaceName); err != nil {
			return nil, err
		}
		itf.gatewayAddress = Network.DefaultIPv6Route.GatewayIP.AsSlice()

	} else {
		return nil, fmt.Errorf("default interface not found")
	}
	return &itf, nil
}

func (m *routeManager) GetRoutableInterface(ifaceName string) (RoutableInterface, error) {

	var (
		err error
	)
	itf := routableInterface{m:m}

	if itf.link, err = netlink.LinkByName(ifaceName); err != nil {
		return nil, err
	}

	// default interface
	if Network.DefaultIPv4Route != nil &&
		Network.DefaultIPv4Route.InterfaceName == ifaceName {

		itf.gatewayAddress = Network.DefaultIPv4Route.GatewayIP.AsSlice()
		return &itf, nil
	}
	if Network.DefaultIPv6Route != nil &&
		Network.DefaultIPv6Route.InterfaceName == ifaceName {

		itf.gatewayAddress = Network.DefaultIPv6Route.GatewayIP.AsSlice()
		return &itf, nil
	}

	// search static routes
	for _, r := range Network.StaticRoutes {
		if r.InterfaceName == ifaceName {
			if r.GatewayIP.IsValid() {

				itf.gatewayAddress = r.GatewayIP.AsSlice()
				return &itf, nil
			}
		}
	}

	return &itf, nil
}

func (m *routeManager) NewRoutableInterface(ifaceName, address string) (RoutableInterface, error) {

	var (
		err error

		ip    net.IP
		ipNet *net.IPNet
	)
	itf := routableInterface{m:m}

	if ip, ipNet, err = net.ParseCIDR(address); err != nil {
		return nil, err
	}
	size, _ := ipNet.Mask.Size()
	if (size == 32) {
		// default to a /24 if address
		// does not indicate network
		ipNet.Mask = net.CIDRMask(24, 32)
	}

	if itf.link, err = netlink.LinkByName(ifaceName); err != nil {
		return nil, err
	}
	ipConfig := &netlink.Addr{IPNet: &net.IPNet{
		IP: ip,
		Mask: ipNet.Mask,
	}}
	if err = netlink.AddrAdd(itf.link, ipConfig); err != nil {
		return nil, err
	}
	if err = netlink.LinkSetUp(itf.link); err != nil {
		return nil, err
	}

	// determine gateway from interface's subnet
	itf.gatewayAddress = ip.Mask(ipNet.Mask);
	IncIP(itf.gatewayAddress)

	m.nc.routedItfs = append(m.nc.routedItfs, itf)
	return &itf, nil
}

func (m *routeManager) AddExternalRouteToIPs(ips []string) error {

	var (
		err error

		destIP net.IP
	)
	gatewayIP := Network.DefaultIPv4Route.GatewayIP.AsSlice()

	for _, ip := range ips {
		if destIP = net.ParseIP(ip); destIP != nil {
			route := netlink.Route{
				Scope:     netlink.SCOPE_UNIVERSE,
				LinkIndex: Network.DefaultIPv4Route.InterfaceIndex,
				Dst:       &net.IPNet{IP: destIP, Mask: net.CIDRMask(32, 32)},
				Gw:        gatewayIP,
			}
			if err = netlink.RouteAdd(&route); err != nil {
				logger.ErrorMessage(
					"routeManager.AddExternalRouteToIPs(): Unable to add static route %s via gateway %s: %s",
					route.Dst, Network.DefaultIPv4Route.GatewayIP.String(), err.Error())
			}	else {
				m.nc.routedIPs = append(m.nc.routedIPs, route)
			}
		}
	}
	return nil
}

func (m *routeManager) AddDefaultRoute(gateway string) error {

	var (
		err error

		gwIP     net.IP
		routes[] netlink.Route
	)

	if gwIP = net.ParseIP(gateway); gwIP == nil {
		return fmt.Errorf("'%s' is not a valid ip", gateway)
	}

	if routes, err = netlink.RouteGet(gwIP); err != nil {
		return err
	}
	if len(routes) > 0 {
		route := routes[0]
		if !route.Gw.Equal(gwIP) {
			return fmt.Errorf(
				"given ip '%s' is not a valid gateway as its route is via '%s'",
				gateway, route.Gw.String(),
			)
		}
		itf := routableInterface{
			gatewayAddress: gwIP,
		}
		if itf.link, err = netlink.LinkByIndex(route.LinkIndex); err != nil {
			return err
		}
		return itf.MakeDefaultRoute()

	} else {
		return fmt.Errorf("no routes found to '%s'", gateway)
	}
}

func (m *routeManager) Clear() {

	var (
		err error
	)

	// remove all packet filter rules
	m.pfr.Clear()

	// down all added interfaces. this will
	// clear any routes via the interface
	if len(m.nc.routedItfs) > 0 {
		for _, itf := range m.nc.routedItfs {
			if err = netlink.LinkSetDown(itf.link); err != nil {
				logger.DebugMessage(
					"routeManager.Clear(): Interface %s down returned message: %s",
					itf.link.Attrs().Name, err.Error())
			}
		}
	}

	// clear routed ips if any
	if len(m.nc.routedIPs) > 0 {
		for _, route := range m.nc.routedIPs {
			if err = netlink.RouteDel(&route); err != nil {
				logger.ErrorMessage(
					"routeManager.Clear(): Unable to delete static route to IP %s: %s",
					route.Dst, err.Error())
			}
		}
		m.nc.routedIPs = nil
	}

	// restore default lan route
	if err = netlink.RouteReplace(&netlink.Route{
		Scope:     netlink.SCOPE_UNIVERSE,
		LinkIndex: Network.DefaultIPv4Route.InterfaceIndex,
		Gw:        Network.DefaultIPv4Route.GatewayIP.AsSlice(),
	}); err != nil {
		logger.ErrorMessage(
			"routeManager.Clear(): Unable to restore default route: %s",
			err.Error())
	}
}

func (i *routableInterface) Name() string {
	return i.link.Attrs().Name
}

func (i *routableInterface) Address4() (string, string, error) {
	a, p, err := i.address(netlink.FAMILY_V4)
	return a.String(), p.String(), err
}

func (i *routableInterface) Address6() (string, string, error) {
	a, p, err := i.address(netlink.FAMILY_V6)
	return a.String(), p.String(), err
}

func (i *routableInterface) address(family int) (netip.Addr, netip.Prefix, error) {

	var (
		err error

		addrs []netlink.Addr
	)

	if addrs, err = netlink.AddrList(i.link, family); err != nil {
		return netip.Addr{}, netip.Prefix{}, err
	}
	if len(addrs) == 0 {
		return netip.Addr{}, netip.Prefix{},
			fmt.Errorf("no addressess found for interface %s", i.link.Attrs().Name)
	}
	addr := addrs[0]

	o, _ := addr.IPNet.Mask.Size()
	a, _ := netip.AddrFromSlice(addr.IPNet.IP)
	p := netip.PrefixFrom(a, o).Masked()

	return a, p, nil
}

func (i *routableInterface) MakeDefaultRoute() error {

	return netlink.RouteReplace(&netlink.Route{
		Scope:     netlink.SCOPE_UNIVERSE,
		LinkIndex: i.link.Attrs().Index,
		Gw:        i.gatewayAddress,
	})
}

func (i *routableInterface) SetSecurityGroups(sgs []SecurityGroup) error {
	return i.m.pfr.SetSecurityGroups(sgs, i.link.Attrs().Name)
}

func (i *routableInterface) DeleteSecurityGroups(sgs []SecurityGroup) error {
	return i.m.pfr.DeleteSecurityGroups(sgs, i.link.Attrs().Name)
}

func (i *routableInterface) ForwardPortTo(proto Protocol, dstPort int, forwardPort int, forwardIP netip.Addr) (string, error) {

	var (
		err error

		dstIP netip.Addr
	)

	if forwardIP.Is4() {
		dstIP, _, err = i.address(netlink.FAMILY_V4)
	} else {
		dstIP, _, err = i.address(netlink.FAMILY_V6)
	}
	if err != nil {
		return "", err
	}

	return i.m.pfr.ForwardPortOnIP(
		dstPort, forwardPort, 
		dstIP, forwardIP, 
		proto,
	)
}

func (i *routableInterface) DeletePortForwardedTo(proto Protocol, dstPort int, forwardPort int, forwardIP netip.Addr) error {

	var (
		err error

		dstIP netip.Addr
	)

	if forwardIP.Is4() {
		dstIP, _, err = i.address(netlink.FAMILY_V4)
	} else {
		dstIP, _, err = i.address(netlink.FAMILY_V6)
	}
	if err != nil {
		return err
	}

	return i.m.pfr.DeleteForwardPortOnIP(
		dstPort, forwardPort, 
		dstIP, forwardIP, 
		proto,
	)
}

func (i *routableInterface) FowardTrafficTo(dstItf RoutableInterface, srcNetwork, dstNetwork string, withNat bool) (string, error) {
	return dstItf.FowardTrafficFrom(i, srcNetwork, dstNetwork, withNat)
}

func (i *routableInterface) DeleteTrafficForwardedTo(dstItf RoutableInterface, srcNetwork, dstNetwork string) error {
	return dstItf.DeleteTrafficForwardedFrom(i, srcNetwork, dstNetwork)
}

func (i *routableInterface) FowardTrafficFrom(srcItf RoutableInterface, srcNetwork, dstNetwork string, withNat bool) (string, error) {

	var (
		err error

		srcNetworkPrefix netip.Prefix
		dstNetworkPrefix netip.Prefix
	)
	srcRitf := srcItf.(*routableInterface)

	if srcNetworkPrefix, err = srcRitf.getNetworkPrefix(srcNetwork); err != nil {
		return "", err
	}
	if dstNetworkPrefix, err = i.getNetworkPrefix(dstNetwork); err != nil {
		return "", err
	}	

	return i.m.pfr.ForwardTraffic(
		srcRitf.link.Attrs().Name,
		i.link.Attrs().Name,
		srcNetworkPrefix,
		dstNetworkPrefix,
		withNat,
	)
}

func (i *routableInterface) DeleteTrafficForwardedFrom(srcItf RoutableInterface, srcNetwork, dstNetwork string) error {

	var (
		err error

		srcNetworkPrefix netip.Prefix
		dstNetworkPrefix netip.Prefix
	)
	srcRitf := srcItf.(*routableInterface)

	if srcNetworkPrefix, err = srcRitf.getNetworkPrefix(srcNetwork); err != nil {
		return err
	}
	if dstNetworkPrefix, err = i.getNetworkPrefix(dstNetwork); err != nil {
		return err
	}

	return i.m.pfr.DeleteForwardTraffic(
		srcRitf.link.Attrs().Name,
		i.link.Attrs().Name,
		srcNetworkPrefix,
		dstNetworkPrefix,
	)
}

func (i *routableInterface) getNetworkPrefix(network string) (networkPrefix netip.Prefix, err error) {
	if network == LAN4 {
		_, networkPrefix, err = i.address(netlink.FAMILY_V4)
	} else if network == LAN6 {
		_, networkPrefix, err = i.address(netlink.FAMILY_V6)
	} else {
		networkPrefix, err = netip.ParsePrefix(network)
	}
	return
}
