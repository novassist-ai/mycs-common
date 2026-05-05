package network

import (
	"fmt"
	"net/netip"
	"time"
)

type Route struct {
	InterfaceIndex int
	InterfaceName  string

	GatewayIP,
	SrcIP,
	DestIP   netip.Addr
	DestCIDR netip.Prefix
	
	IsIPv6            bool
	IsInterfaceScoped bool
}

// global network properties
var Network = struct {

	// default interface and gateway for 
	// all WAN traffic (i.e. 0.0.0.0/0 & ::/0)
	DefaultIPv4Route *Route
	DefaultIPv6Route *Route

	// additional default routes scoped
	// to specific interfaces
  ScopedDefaults []*Route

	// all static routes
	StaticRoutes []*Route
}{}

var (
	initErr     chan error
	initialized bool

	prefixWorld4 = netip.MustParsePrefix(WORLD4)
	prefixWorld6 = netip.MustParsePrefix(WORLD6)
)

// route type functions

func (r *Route) String() string {
	if r.GatewayIP.IsValid() {
		return fmt.Sprintf(
			"%s via %s on interface %s (ip: %s, scoped: %t)",
			r.DestCIDR, r.GatewayIP, r.InterfaceName, r.SrcIP, r.IsInterfaceScoped,
		)	
	} else {
		return fmt.Sprintf(
			"%s on interface %s (ip: %s, scoped: %t)",
			r.DestCIDR, r.InterfaceName, r.SrcIP, r.IsInterfaceScoped,
		)
	}
}

// network context type common functions

func (c *networkContext) DefaultInterface() string {
	return Network.DefaultIPv4Route.InterfaceName
}

func (c *networkContext) DefaultGateway() string {
	return Network.DefaultIPv4Route.GatewayIP.String()
}

func (c *networkContext) DefaultIP() string {
	return Network.DefaultIPv4Route.SrcIP.String()
}

// commong network context initialization functions

func waitForInit() error {

	var (
		err error
	)

	if !initialized {
		select {
		case err = <-initErr:
			initialized = true
		case <-time.After(time.Second * 30):
			err = fmt.Errorf("timedout waiting for network to complete initialization")
		}		
	}
	return err
}

func init() {
	initErr = make(chan error)
	initialized = false
}
