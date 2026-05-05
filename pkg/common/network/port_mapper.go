package network

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/huin/goupnp/dcps/internetgateway2"
	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/novassist/mycs-common/pkg/goutils/utils"
	"golang.org/x/sync/errgroup"
)

type Protocol string

const (
	ProtocolTCP Protocol = "TCP"
	ProtocolUDP Protocol = "UDP"
)

type PortMapper interface {	
	Connect(timeout time.Duration) error
	Close()

	ExternalIP() string
	LocalIP() string

	AddPersistantPortMappingToSelf(
		description string,
		protocol Protocol,
		externalPort uint16, 
		forwardToPort uint16, 
	) error
	AddPersistantPortMapping(
		description string,
		protocol Protocol,
		externalPort uint16, 
		forwardToPort uint16, 
		forwardToAddr netip.Addr,
	) error
	AddPortMappingToSelf(
		description string,
		protocol Protocol,
		externalPort uint16, 
		forwardToPort uint16, 
		timeout time.Duration,
	) error
	AddPortMapping(
		description string,
		protocol Protocol,
		externalPort uint16, 
		forwardToPort uint16, 
		forwardToAddr netip.Addr,
		timeout time.Duration,
	) error
}

type portMapper struct {
	ctx context.Context

	upnpClient upnpClient

	externalAddr netip.Addr
	selfAddr     netip.Addr

	pRefreshTimer    *utils.ExecTimer
	pRefreshInterval time.Duration
	pPortMappings    []pPortMapping

	pPortMappingTimeout time.Duration

	mx sync.Mutex
}

type pPortMapping struct {
	description   string
	protocol      Protocol
	externalPort  uint16
	forwardToPort uint16
	forwardToAddr netip.Addr
}

type upnpClient interface {
	AddPortMappingCtx(
		ctx context.Context,
		NewRemoteHost string,
		NewExternalPort uint16,
		NewProtocol string,
		NewInternalPort uint16,
		NewInternalClient string,
		NewEnabled bool,
		NewPortMappingDescription string,
		NewLeaseDuration uint32,	// in seconds
	) (err error)

	GetExternalIPAddressCtx(ctx context.Context) (
		NewExternalIPAddress string,
		err error,
	)

	LocalAddr() net.IP
}

var (
	ErrMultipleRoutesFound = errors.New("found multiple routes in the network")
	ErrNoRoutersFound = errors.New("no routers offering upnp services found")
)

func NewPortMapper(
	ctx context.Context,
	pRefresh time.Duration, // in millis
) PortMapper {

	p := &portMapper{
		ctx: ctx,
		pRefreshInterval:    pRefresh,
		pPortMappingTimeout: (pRefresh * time.Millisecond) + time.Minute,
	}
	p.pRefreshTimer = utils.NewExecTimer(p.ctx, p.refreshPortMappings, false)

	return p
}

func (p *portMapper) Connect(timeout time.Duration) error {

	var (
		err error

		routerEIP, routerEIPn string
	)

	ctx, cancelFunc := context.WithTimeout(p.ctx, timeout)
	defer cancelFunc()

	tasks, _ := errgroup.WithContext(p.ctx)
	// Request each type of client in parallel, and return what is found.
	var ip1Clients []*internetgateway2.WANIPConnection1
	tasks.Go(func() error {
		var err error
		ip1Clients, _, err = internetgateway2.NewWANIPConnection1Clients()
		return err
	})
	var ip2Clients []*internetgateway2.WANIPConnection2
	tasks.Go(func() error {
		var err error
		ip2Clients, _, err = internetgateway2.NewWANIPConnection2Clients()
		return err
	})
	var ppp1Clients []*internetgateway2.WANPPPConnection1
	tasks.Go(func() error {
		var err error
		ppp1Clients, _, err = internetgateway2.NewWANPPPConnection1Clients()
		return err
	})

	if err = tasks.Wait(); err != nil {
		return err
	}
	
	switch {
	case len(ip1Clients) > 1:
		if routerEIP, err = ip1Clients[0].GetExternalIPAddressCtx(ctx); err != nil {
			return err
		}
		for i := 1; i < len(ip1Clients); i++ {
			if routerEIPn, err = ip1Clients[i].GetExternalIPAddressCtx(ctx); err == nil && routerEIPn != routerEIP {
				return ErrMultipleRoutesFound
			}
		}
		p.upnpClient = ip1Clients[0]

	case len(ip2Clients) > 1:		
		if routerEIP, err = ip2Clients[0].GetExternalIPAddressCtx(ctx); err != nil {
			return err
		}
		for i := 1; i < len(ip2Clients); i++ {
			if routerEIPn, err = ip2Clients[i].GetExternalIPAddressCtx(ctx); err == nil && routerEIPn != routerEIP {
				return ErrMultipleRoutesFound
			}
		}
		p.upnpClient = ip2Clients[0]
		
	case len(ppp1Clients) > 1:
		if routerEIP, err = ppp1Clients[0].GetExternalIPAddressCtx(ctx); err != nil {
			return err
		}
		for i := 1; i < len(ppp1Clients); i++ {
			if routerEIPn, err = ppp1Clients[i].GetExternalIPAddressCtx(ctx); err == nil && routerEIPn != routerEIP {
				return ErrMultipleRoutesFound
			}
		}
		p.upnpClient = ppp1Clients[0]

	default:
		return ErrNoRoutersFound
	}

	p.externalAddr = netip.MustParseAddr(routerEIP)
	p.selfAddr = netip.MustParseAddr(p.upnpClient.LocalAddr().String())
	
	if err = p.pRefreshTimer.Start(0); err == nil {
		return err
	}
	return nil
}

func (p *portMapper) Close() {	

	// stop the port refresh timer
	if err := p.pRefreshTimer.Stop(); err != nil {
		logger.ErrorMessage(
			"portMapper.Close(): Port refresh timer stopped with err: %s", 
			err.Error(),
		)
	}
}

func (p *portMapper) refreshPortMappings() (time.Duration, error) {

	var (
		err error
	)

	p.mx.Lock()
	defer p.mx.Unlock()

	for _, pm := range p.pPortMappings {
		if err = p.AddPortMapping(
			pm.description,
			pm.protocol,
			pm.externalPort, 
			pm.forwardToPort, 
			pm.forwardToAddr, 
			p.pPortMappingTimeout,
		); err != nil {
			logger.ErrorMessage(
				"portMapper.refreshPortMappings(): Failed to refresh port mapping '%+v': %s", 
				pm, err.Error(),
			)
		}
	}	
	return p.pRefreshInterval, nil
}

func (p *portMapper) ExternalIP() string {
	return p.externalAddr.String()
}

func (p *portMapper) LocalIP() string {
	return p.selfAddr.String()
}

func (p *portMapper) AddPersistantPortMappingToSelf(
	description string,
	protocol Protocol,
	externalPort uint16, 
	forwardToPort uint16, 
) error {
	return p.AddPersistantPortMapping(
		description, 
		protocol, 
		externalPort, 
		forwardToPort, 
		p.selfAddr,
	)
}

func (p *portMapper) AddPersistantPortMapping(
	description string,
	protocol Protocol,
	externalPort uint16, 
	forwardToPort uint16, 
	forwardToAddr netip.Addr,
) error {

	var (
		err error
	)

	if err = p.AddPortMapping(
		description,
		protocol,
		externalPort, 
		forwardToPort, 
		forwardToAddr, 
		p.pPortMappingTimeout,
	); err != nil {
		return err
	}

	p.mx.Lock()
	defer p.mx.Unlock()
	p.pPortMappings = append(
		p.pPortMappings, 
		pPortMapping{ 
			description:   description,
			protocol:      protocol,
			externalPort:  externalPort,
			forwardToPort: forwardToPort,
			forwardToAddr: forwardToAddr,
		},
	)

	return nil
}

func (p *portMapper) AddPortMappingToSelf(
	description string,
	protocol Protocol,
	externalPort uint16, 
	forwardToPort uint16, 
	timeout time.Duration,
) error {
	return p.AddPortMapping(
		description, 
		protocol, 
		externalPort, 
		forwardToPort, 
		p.selfAddr, 
		timeout,
	)
}

func (p *portMapper) AddPortMapping(
	description string,
	protocol Protocol,
	externalPort uint16, 
	forwardToPort uint16, 
	forwardToAddr netip.Addr,
	timeout time.Duration,
) error {
	
	return p.upnpClient.AddPortMappingCtx(
		p.ctx,
		"",						                 // NewRemoteHost
		externalPort,                  // NewExternalPort
		string(protocol),              // NewProtocol
		forwardToPort,                 // NewInternalPort
		forwardToAddr.String(),        // NewInternalClient
		true,                          // NewEnabled
		description,                   // NewPortMappingDescription
		uint32(timeout / time.Second), // NewLeaseDuration (secs)
	)
}
