//go:build windows

package network

type networkContext struct {
}

func NewNetworkContext() (NetworkContext, error) {
	return &networkContext{}, nil
}

func (c *networkContext) DefaultDeviceName() string {
	return Network.DefaultIPv4Route.InterfaceName
}

func (c *networkContext) DisableIPv6() error {
	return nil
}

func (c *networkContext) Clear() {
}
