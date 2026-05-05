//go:build windows

package network

type dnsManager struct {
	nc *networkContext
}

func (c *networkContext) NewDNSManager() (DNSManager, error) {
	
	m := &dnsManager{
		nc: c,
	}
	return m, nil
}

func (m *dnsManager) AddDNSServers(servers []string) error {
	return nil
}

func (m *dnsManager) AddSearchDomains(domains []string) error {
	return nil
}

func (m *dnsManager) Clear() {
}
