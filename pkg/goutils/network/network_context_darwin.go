//go:build darwin

package network

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"syscall"
	"time"

	"github.com/mitchellh/go-homedir"
	netroute "golang.org/x/net/route"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/novassist/mycs-common/pkg/goutils/run"
	"github.com/novassist/mycs-common/pkg/goutils/utils"
)

type networkContext struct { 
	ipv6Disabled bool

	origDNSServers    []string
	origSearchDomains []string

	routedIPs []string
}

var (
	ifconfig, 
	route,
	routeget,
	pfctl,
	networksetup run.CLI

	netServiceName string
)

func NewNetworkContext() (NetworkContext, error) {

	if err := waitForInit(); err != nil {
		return nil, err
	}

	return &networkContext{ 
		origDNSServers:    []string{ "empty" },
		origSearchDomains: []string{ "empty" },
	}, nil
}

func (c *networkContext) DefaultDeviceName() string {
	return netServiceName
}

func (c *networkContext) DisableIPv6() error {
	if err := networksetup.Run([]string{ "-setv6off", netServiceName }); err != nil {
		logger.ErrorMessage("networkContext.DisableIPv6(): Error running \"networksetup -setv6off %s\": %s", netServiceName, err.Error())
		return err
	}
	c.ipv6Disabled = true
	return nil
}

func (c *networkContext) Clear() {
	
	var (
		err error

		dnsManager   DNSManager
		routeManager RouteManager
	)

	if c.ipv6Disabled {
		if err := networksetup.Run([]string{ "-setv6automatic", netServiceName }); err != nil {
			logger.ErrorMessage("networkContext.DisableIPv6(): Error running \"networksetup -setv6automatic %s\": %s", netServiceName, err.Error())
		}	else {
			c.ipv6Disabled = false
		}
	}

	if dnsManager, err = c.NewDNSManager(); err != nil {
		logger.ErrorMessage(
			"networkContext.Clear(): Error creating DNS manager to use to clear network context: %s", 
			err.Error(),
		)
	}
	dnsManager.Clear()

	if routeManager, err = c.NewRouteManager(); err != nil {
		logger.ErrorMessage(
			"networkContext.Clear(): Error creating DNS manager to use to clear network context: %s", 
			err.Error(),
		)
	}
	routeManager.Clear()
}

func init() {

	var (
		err error
	)

	// initialize networking CLIs
	go func() {
		home, _ := homedir.Dir()
		if ifconfig, err = run.NewCLI("/sbin/ifconfig", home, nullOut, nullOut); err != nil {
			logger.ErrorMessage("networkContext.init(): Error creating CLI for /sbin/ifconfig: %s", err.Error())
			initErr <- err
			return
		}
		if route, err = run.NewCLI("/sbin/route", home, nullOut, nullOut); err != nil {
			logger.ErrorMessage("networkContext.init(): Error creating CLI for /sbin/route: %s", err.Error())
			initErr <- err
			return
		}
		if routeget, err = run.NewCLI("/sbin/route", home, &outputBuffer, &outputBuffer); err != nil {
			logger.ErrorMessage("networkContext.init(): Error creating CLI for /sbin/route: %s", err.Error())
			initErr <- err
			return
		}
		if pfctl, err = run.NewCLI("/sbin/pfctl", home, &outputBuffer, &outputBuffer); err != nil {
			logger.ErrorMessage("networkContext.init(): Error creating CLI for /sbin/pfctl: %s", err.Error())
			initErr <- err
			return
		}
		if networksetup, err = run.NewCLI("/usr/sbin/networksetup", home, &outputBuffer, &outputBuffer); err != nil {
			logger.ErrorMessage("networkContext.init(): Error creating CLI for /usr/sbin/networksetup: %s", err.Error())
			initErr <- err
			return
		}
	
		readNetworkInfo()
	}()
}

func readNetworkInfo() {

	var (
		err error

		results map[string][][]string		
		line    string
	)

	Network.ScopedDefaults = nil
	Network.StaticRoutes = nil

	// read network routing details
	if err = readRouteTable(); err != nil {
		initErr <- err
		return
	}
	if Network.DefaultIPv4Route == nil {
		// enumerate all network service interfaces
		if err = networksetup.Run([]string{ "-listnetworkserviceorder" }); err != nil {
			logger.ErrorMessage("networkContext.init(): Error running \"networksetup -listnetworkserviceorder\": %s", err.Error())
			initErr <- err
			return
		}
		results = utils.ExtractMatches(outputBuffer.Bytes(), map[string]*regexp.Regexp{
			"ports": regexp.MustCompile(`^\(Hardware Port: .* Device: ([a-z]+[0-9]*)\)$`),
		})
		outputBuffer.Reset()

		// restart each network interface
		for _, p := range results["ports"] {
			if len(p) == 2 {
				if err = run.RunAsAdminWithArgs([]string{"ifconfig", p[1], "down"}, &outputBuffer, &outputBuffer); err == nil {
					_ = run.RunAsAdminWithArgs([]string{"ifconfig", p[1], "up"}, &outputBuffer, &outputBuffer)
				}
			}
		}
		// allow network service to re-initialize route table
		time.Sleep(5 * time.Second)

		if err = readRouteTable(); err != nil {
			initErr <- err
			return
		}
		if Network.DefaultIPv4Route == nil {
			logger.ErrorMessage("networkContext.init(): Unable to determine the default gateway. Please restart you systems network services.")
			initErr <- err
			return
		}
	}

	// determine network service name for default device
	if err = routeget.Run([]string{ "get", "1.1.1.1" }); err != nil {
		logger.DebugMessage("networkContext.init(): Error running \"route get 1.1.1.1\": %s", err.Error())
		initErr <- err
		return
	}
	results = utils.ExtractMatches(outputBuffer.Bytes(), map[string]*regexp.Regexp{
		"interface": regexp.MustCompile(`^\s*interface:\s*(.*)\s*$`),
	})
	outputBuffer.Reset()

	lanGatewayItf := Network.DefaultIPv4Route.InterfaceName
	defaultItf := results["interface"]	
	if len(defaultItf) > 0 && len(defaultItf[0]) == 2 {
		lanGatewayItf = defaultItf[0][1]
	}
	
	if err = networksetup.Run([]string{ "-listallhardwareports" }); err != nil {
		logger.ErrorMessage("networkContext.init(): Error running \"networksetup -listallhardwareports\": %s", err.Error())
		initErr <- err
		return
	}
	matchDevice := "Device: " + lanGatewayItf
	prevLine := ""
	scanner := bufio.NewScanner(bytes.NewReader(outputBuffer.Bytes()))
	for scanner.Scan() {
		line = scanner.Text()
		if line == matchDevice && len(prevLine) > 0 {
			netServiceName = prevLine[15:]
			break;
		}
		prevLine = line
	}
	outputBuffer.Reset()

	if len(netServiceName) == 0 {
		initErr <- fmt.Errorf(
			"unable to determine default network service name for for interface \"%s\"", 
			Network.DefaultIPv4Route.InterfaceName,
		)
		return
	}

	initErr <- nil
}

func readRouteTable() error {

	var (
		err error
		ok  bool

		rib     []byte
		msgs    []netroute.Message
		rm      *netroute.RouteMessage
		iface   *net.Interface
		netMask netip.Addr
	)

	// retrieve network route information by querying the system's routing table
	// ref syscall constants: https://github.com/apple/darwin-xnu/blob/main/bsd/net/route.h

	if rib, err = netroute.FetchRIB(syscall.AF_UNSPEC, syscall.NET_RT_DUMP2, 0); err != nil {
		logger.ErrorMessage("networkContext.init(): Error fetching system route table: %s", err.Error())
		return err
	}
	if msgs, err = netroute.ParseRIB(syscall.NET_RT_IFLIST2, rib); err != nil {
		logger.ErrorMessage("networkContext.init(): Error parsing fetched route table data: %s", err.Error())
		return err
	}

	defaultIPv4Route := netip.MustParsePrefix("0.0.0.0/0")
	defaultIPv6Route := netip.MustParsePrefix("::/0")

	var getAddr = func(addr netroute.Addr) (netip.Addr, bool, bool) {
		if addr != nil {
			switch addr.Family() {
			case syscall.AF_INET:
				return netip.AddrFrom4(addr.(*netroute.Inet4Addr).IP), false, true
			case syscall.AF_INET6:
				return netip.AddrFrom16(addr.(*netroute.Inet6Addr).IP), true, true
			}	
		}
		return netip.Addr{}, false, false
	}

	for _, m := range msgs {
		if rm, ok = m.(*netroute.RouteMessage); !ok {
			continue
		}
		if rm.Flags & syscall.RTF_UP == 0 || 
			rm.Flags & syscall.RTF_GATEWAY == 0 || 
			rm.Flags & syscall.RTF_WASCLONED != 0 || 
			len(rm.Addrs) == 0 {
			continue
		}
		if iface, err = net.InterfaceByIndex(rm.Index); err != nil {
			logger.ErrorMessage(
				"networkContext.init(): Error looking up interface for index %d: %s",
				rm.Index,
				err.Error(),
			)
			continue
		}
		r := &Route{
			InterfaceIndex: rm.Index,
			InterfaceName:  iface.Name,
		}
		if r.GatewayIP, r.IsIPv6, ok = getAddr(rm.Addrs[syscall.RTAX_GATEWAY]); !ok {
			logger.ErrorMessage("networkContext.init(): Gateway address not present for route message: %# v", rm)
			continue
		}
		if r.SrcIP, _, ok = getAddr(rm.Addrs[syscall.RTAX_IFA]); !ok {
			logger.ErrorMessage("networkContext.init(): Source address not present for route message: %# v", rm)
			continue
		}
		if r.DestIP, _, ok = getAddr(rm.Addrs[syscall.RTAX_DST]); !ok {
			logger.ErrorMessage("networkContext.init(): Destination address not present for route message: %# v", rm)
			continue
		}
		if netMask, _, ok = getAddr(rm.Addrs[syscall.RTAX_NETMASK]); !ok {
			logger.DebugMessage("networkContext.init(): Broadcast address not present for route message: %# v", rm)
		}
		ones, _ := net.IPMask(netMask.AsSlice()).Size()
		r.DestCIDR = netip.PrefixFrom(r.DestIP, ones)
		
		r.IsInterfaceScoped = rm.Flags & syscall.RTF_IFSCOPE != 0
		if r.IsIPv6 {
			if r.DestCIDR == defaultIPv6Route {
				if !r.IsInterfaceScoped {
					if Network.DefaultIPv6Route == nil {
						Network.DefaultIPv6Route = r
					}	else {
						logger.ErrorMessage("networkContext.init(): Duplicate default ip v6 route will be ignored: %# v", r)
					}		
				} else {
					Network.ScopedDefaults = append(Network.ScopedDefaults, r)
				}
			} else {
				Network.StaticRoutes = append(Network.StaticRoutes, r)
			}
		} else {
			if r.DestCIDR == defaultIPv4Route {
				if !r.IsInterfaceScoped {
					if Network.DefaultIPv4Route == nil {
						Network.DefaultIPv4Route = r
					}	else {
						logger.ErrorMessage("networkContext.init(): Duplicate default ip v4 route will be ignored: %# v", r)
					}
				} else {
					Network.ScopedDefaults = append(Network.ScopedDefaults, r)
				}
			} else {
				Network.StaticRoutes = append(Network.StaticRoutes, r)
			}
		}
	}

	return nil
}
