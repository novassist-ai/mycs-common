//go:build windows

package network

import (
	"fmt"
	"net/netip"
)

type routeManager struct {	
	nc *networkContext
}

type routableInterface struct {
	gatewayAddress string
}

func (c *networkContext) NewRouteManager() (RouteManager, error) {

	rm := &routeManager{
		nc: c,
	}
	return rm, nil
}

func (m *routeManager) GetDefaultInterface() (RoutableInterface, error) {
	return nil, nil
}

func (m *routeManager) GetRoutableInterface(ifaceName string) (RoutableInterface, error) {
	return nil, nil
}

func (m *routeManager) NewRoutableInterface(ifaceName, address string) (RoutableInterface, error) {
	return &routableInterface{}, nil
}

func (m *routeManager) NewFilterRouter(denyAll bool) (FilterRouter, error) {
	return nil, fmt.Errorf("filter router has not been implemented for windows os")
}

func (m *routeManager) AddExternalRouteToIPs(ips []string) error {
	return nil
}

func (m *routeManager) AddDefaultRoute(gateway string) error {
	return nil
}

func (m *routeManager) Clear() {
}

func (i *routableInterface) Name() string {
	return ""
}

func (i *routableInterface) Address4() (string, string, error) {
	return "", "", nil
}

func (i *routableInterface) Address6() (string, string, error) {
	return "", "", nil
}

func (i *routableInterface) MakeDefaultRoute() error {
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
	return "", nil
}

func (i *routableInterface) DeleteTrafficForwardedTo(dstItf RoutableInterface, srcNetwork, dstNetwork string) error {
	return nil
}

func (i *routableInterface) FowardTrafficFrom(srcItf RoutableInterface, srcNetwork, dstNetwork string, withNat bool) (string, error) {
	return "", nil
}

func (i *routableInterface) DeleteTrafficForwardedFrom(srcItf RoutableInterface, srcNetwork, destNetwork string) error {
	return nil
}
