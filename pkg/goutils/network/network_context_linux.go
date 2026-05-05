//go:build linux

package network

import (
	"fmt"
	"net"
	"net/netip"

	"github.com/vishvananda/netlink"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
)

type networkContext struct {
	routedItfs []routableInterface
	routedIPs  []netlink.Route

	dm *dnsManager
	rm *routeManager
}

func NewNetworkContext() (NetworkContext, error) {

	if err := waitForInit(); err != nil {
		return nil, err
	}

	return &networkContext{}, nil
}

func (c *networkContext) DefaultDeviceName() string {
	return Network.DefaultIPv4Route.InterfaceName
}

func (c *networkContext) DisableIPv6() error {
	return nil
}

func (c *networkContext) Clear() {
	
	if c.dm != nil {
		c.dm.Clear()
	}
	if c.rm != nil {
		c.rm.Clear()
	}
}

func init() {
	go readNetworkInfo()
}

func readNetworkInfo() {

	var (
		err error

		routes []netlink.Route
	)

	Network.ScopedDefaults = nil
	Network.StaticRoutes = nil

	if routes, err = netlink.RouteList(nil, netlink.FAMILY_V4); err != nil {
		logger.ErrorMessage("networkContext.init(): Error looking up ipv4 routes: %s", err.Error())
		initErr <- err
		return
	}
	if err = readRoutes(
		netip.MustParseAddr("0.0.0.0"), 
		netip.MustParsePrefix("0.0.0.0/0"), 
		routes,
		netlink.FAMILY_V4,
	); err != nil {
		initErr <- err
		return
	}

	if routes, err = netlink.RouteList(nil, netlink.FAMILY_V6); err != nil {
		logger.ErrorMessage("networkContext.init(): Error looking up ipv6 routes: %s", err.Error())
		initErr <- err
		return
	}
	if err = readRoutes(
		netip.MustParseAddr("::"),
		netip.MustParsePrefix("::/0"),
		routes,
		netlink.FAMILY_V6,
	); err != nil {
		initErr <- err
		return
	}

	if Network.DefaultIPv4Route == nil {
		initErr <- fmt.Errorf("unable to determine default network interface and gateway")
		return
	}

	initErr <- nil
}

func readRoutes(
	defaultRouteIP netip.Addr, 
	defaultRouteCIDR netip.Prefix,  
	routes []netlink.Route,
	family int,
) error {

	var (
		err error
		ok  bool

		iface  *net.Interface
		addrs  []net.Addr
		prefix netip.Prefix
	)

	for _, route := range routes {
		if iface, err = net.InterfaceByIndex(route.LinkIndex); err != nil {
			logger.ErrorMessage(
				"networkContext.readRoutes(): Error looking up interface for index %d: %s",
				route.LinkIndex,
				err.Error(),
			)
			return err
		}
		r := &Route{
			InterfaceIndex:    route.LinkIndex,
			InterfaceName:     iface.Name,
			IsInterfaceScoped: route.Scope == netlink.SCOPE_LINK,
		}
		if route.Gw != nil {
			if r.GatewayIP, ok = netip.AddrFromSlice(route.Gw); !ok {
				logger.ErrorMessage("networkContext.readRoutes(): Error invalid gateway IP: %s", err.Error())
				continue
			}
		}		
		if route.Src != nil {
			if r.SrcIP, ok = netip.AddrFromSlice(route.Src); !ok {
				logger.ErrorMessage("networkContext.readRoutes(): Error invalid source IP: %s", err.Error())
				continue
			}
		} else {
			if addrs, err = iface.Addrs(); err != nil {
				logger.ErrorMessage(
					"networkContext.readRoutes(): Unable to retrieve addresses of interface '%s': %s", 
					iface.Name, err.Error(),
				)
			} else if len(addrs) > 0 {
				if prefix, err = netip.ParsePrefix(addrs[0].String()); err != nil {
					logger.ErrorMessage("networkContext.readRoutes(): Error invalid source IP: %s", err.Error())
				} else {
					r.SrcIP = prefix.Addr()
				}
			}
		}
		if route.Dst == nil {
			r.DestIP = defaultRouteIP
			r.DestCIDR = defaultRouteCIDR

			if family == netlink.FAMILY_V4 {
				Network.DefaultIPv4Route = r
			} else {
				Network.DefaultIPv6Route = r
			}

		} else {
			if r.DestIP, ok = netip.AddrFromSlice(route.Dst.IP); !ok {
				logger.ErrorMessage("networkContext.readRoutes(): Error invalid destination CIDR: %s", err.Error())
				continue
			}
			ones, _ := route.Dst.Mask.Size()
			r.DestCIDR = netip.PrefixFrom(r.DestIP, ones)
			Network.StaticRoutes = append(Network.StaticRoutes, r)
		}
	}

	return nil
}
