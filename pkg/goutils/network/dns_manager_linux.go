//go:build linux

package network

import (
	"context"
	"net/netip"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
)

type dnsManager struct {
	nc       *networkContext
	resolved dbus.BusObject
}

// DBus entities we talk to.
//
// DBus is an RPC bus. In particular, the bus we're talking to is the
// system-wide bus (there is also a per-user session bus for
// user-specific applications).
//
// Daemons connect to the bus, and advertise themselves under a
// well-known object name. That object exposes paths, and each path
// implements one or more interfaces that contain methods, properties,
// and signals.
//
// Clients connect to the bus and walk that same hierarchy to invoke
// RPCs, get/set properties, or listen for signals.
const (
	dbusResolvedObject                    = "org.freedesktop.resolve1"
	dbusResolvedPath      dbus.ObjectPath = "/org/freedesktop/resolve1"
	dbusResolvedInterface                 = "org.freedesktop.resolve1.Manager"
	dbusPath              dbus.ObjectPath = "/org/freedesktop/DBus"
	dbusInterface                         = "org.freedesktop.DBus"
	dbusOwnerSignal                       = "NameOwnerChanged" // broadcast when a well-known name's owning process changes.
)

type resolvedLinkNameserver struct {
	Family  int32
	Address []byte
}

type resolvedLinkDomain struct {
	Domain      string
	RoutingOnly bool
}

func (c *networkContext) NewDNSManager() (DNSManager, error) {

	var (
		err error

		conn *dbus.Conn
	)

	if conn, err = dbus.SystemBus(); err != nil {
		return nil, err
	}

	dm := &dnsManager{
		nc:       c,
		resolved: conn.Object(dbusResolvedObject, dbus.ObjectPath(dbusResolvedPath)),
	}
	c.dm = dm
	return c.dm, nil
}

func (m *dnsManager) AddDNSServers(servers []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()

	var (
		err error

		ip netip.Addr
	)

	if len(m.nc.routedItfs) > 0 {
		// configure DNS on first added link
		itf := m.nc.routedItfs[0]
		
		var linkNameservers = make([]resolvedLinkNameserver, len(servers))
		for i, server := range servers {
			if ip, err = netip.ParseAddr(server); err != nil {
				logger.ErrorMessage(
					"dnsManager.AddDNSServers(): Error parsing DNS server '%s': %s", 
					server, err.Error(),
				)
				continue
			}
			nsIPAddr := ip.As16()
			if ip.Is4() {
				linkNameservers[i] = resolvedLinkNameserver{
					Family:  unix.AF_INET,
					Address: nsIPAddr[12:],
				}
			} else {
				linkNameservers[i] = resolvedLinkNameserver{
					Family:  unix.AF_INET6,
					Address: nsIPAddr[:],
				}
			}
		}
	
		if err = m.resolved.CallWithContext(
			ctx, dbusResolvedInterface+".SetLinkDNS", 0,
			itf.link.Attrs().Index, linkNameservers,
		).Store(); err != nil {
			logger.ErrorMessage(
				"dnsManager.AddDNSServers(): Error setting DNS server on link '%s': %s", 
				itf.link.Attrs().Name, err.Error(),
			)
		}	
		// set link as new default DNS route
		if err = m.resolved.CallWithContext(
			ctx, dbusResolvedInterface+".SetLinkDefaultRoute", 0,
			itf.link.Attrs().Index, true,
		).Store(); err != nil {
			logger.ErrorMessage(
				"dnsManager.AddDNSServers(): Error setting DNS configured on link '%s' as default: %s", 
				itf.link.Attrs().Name, err.Error(),
			)
		}
		if err = m.resolved.CallWithContext(ctx, dbusResolvedInterface+".FlushCaches", 0).Err; err != nil {
			logger.ErrorMessage(
				"dnsManager.AddDNSServers(): Failed to flush DNS cache: %s", 
				itf.link.Attrs().Name, err.Error(),
			)
		}
	}

	return nil
}

func (m *dnsManager) AddSearchDomains(domains []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()

	var (
		err error
	)

	if len(m.nc.routedItfs) > 0 {
		// configure DNS search domains on first added link
		itf := m.nc.routedItfs[0]

		var addTrailingDot = func(domain string) string {
			if strings.HasSuffix(domain, ".") {
				return domain
			} else {
				return domain + "."
			}
		}
		
		var linkDomains = make([]resolvedLinkDomain, 0, len(domains) + 1)
		for _, domain := range domains {
			linkDomains = append(linkDomains, resolvedLinkDomain{
				Domain: addTrailingDot(domain),
				RoutingOnly: true,
			})
		}
		linkDomains = append(linkDomains, resolvedLinkDomain{
			Domain: ".",
			RoutingOnly: true,
		})

		if err = m.resolved.CallWithContext(
			ctx, dbusResolvedInterface+".SetLinkDomains", 0,
			itf.link.Attrs().Index, linkDomains,
		).Store(); err != nil {
			logger.ErrorMessage(
				"dnsManager.AddSearchDomains(): Error setting DNS search domain on link '%s': %s", 
				itf.link.Attrs().Name, err.Error(),
			)
		}	
	}
	
	return nil
}

func (m *dnsManager) Clear() {
	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()

	var (
		err error
	)

	if len(m.nc.routedItfs) > 0 {
		// reset DNS configurations for links
		for _, itf := range m.nc.routedItfs {
			if err = m.resolved.CallWithContext(
				ctx, dbusResolvedInterface+".RevertLink", 0, 
				itf.link.Attrs().Index,
			).Err; err != nil {
				logger.DebugMessage(
					"dnsManager.Clear(): Error reverting DNS settings on link %s: %s", 
					itf.link.Attrs().Name, err.Error(),
				)
			}	
		}
	}
}
