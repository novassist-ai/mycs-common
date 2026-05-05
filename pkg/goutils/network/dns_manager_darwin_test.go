//go:build darwin

package network_test

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/novassist/mycs-common/pkg/goutils/network"
	"github.com/novassist/mycs-common/pkg/goutils/run"
	"github.com/mitchellh/go-homedir"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DNS Manager", func() {
	
	var (
		err error

		networksetup run.CLI
		outputBuffer bytes.Buffer

		nc network.NetworkContext
	)

	resetDNSSetting := func() {
		// reset dns settings
		err = networksetup.Run([]string{ "-setdnsservers", nc.DefaultDeviceName(), "empty" })
		Expect(err).NotTo(HaveOccurred())
		outputBuffer.Reset()
		err = networksetup.Run([]string{ "-setsearchdomains", nc.DefaultDeviceName(), "empty" })
		Expect(err).NotTo(HaveOccurred())
		outputBuffer.Reset()
	}

	BeforeEach(func() {
		home, _ := homedir.Dir()
		networksetup, err = run.NewCLI("/usr/sbin/networksetup", home, &outputBuffer, &outputBuffer)
		Expect(err).NotTo(HaveOccurred())

		nc, err = network.NewNetworkContext()
		Expect(err).NotTo(HaveOccurred())
		
		resetDNSSetting()
	})

	AfterEach(func() {
		resetDNSSetting()
	})

	testDNSSetting := func(nc network.NetworkContext, dnsManager network.DNSManager) {
		// set test dns settings
		err = dnsManager.AddDNSServers([]string{ "192.168.100.1", "192.168.100.2" })
		Expect(err).NotTo(HaveOccurred())
		err = dnsManager.AddSearchDomains([]string{ "acme.com", "bugsy.com" })
		Expect(err).NotTo(HaveOccurred())
	
		// verify changes were made
		err = networksetup.Run([]string{ "-getdnsservers", nc.DefaultDeviceName() })
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Fields(outputBuffer.String())).To(Equal([]string{ "192.168.100.1", "192.168.100.2" }))
		outputBuffer.Reset()
		err = networksetup.Run([]string{ "-getsearchdomains", nc.DefaultDeviceName() })
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Fields(outputBuffer.String())).To(Equal([]string{ "acme.com", "bugsy.com" }))
		outputBuffer.Reset()

		dnsManager.Clear()
	}

	It("sets dns and search domain where dns and search domains have not been explicitly configured", func() {
		dnsManager, err := nc.NewDNSManager()
		Expect(err).NotTo(HaveOccurred())

		// set test dns settings
		testDNSSetting(nc, dnsManager)

		// verify changes were restored
		err = networksetup.Run([]string{ "-getdnsservers", nc.DefaultDeviceName() })
		Expect(err).NotTo(HaveOccurred())
		Expect(outputBuffer.String()).To(Equal(fmt.Sprintf("There aren't any DNS Servers set on %s.\n", nc.DefaultDeviceName())))
		outputBuffer.Reset()
		err = networksetup.Run([]string{ "-getsearchdomains", nc.DefaultDeviceName() })
		Expect(err).NotTo(HaveOccurred())
		Expect(outputBuffer.String()).To(Equal(fmt.Sprintf("There aren't any Search Domains set on %s.\n", nc.DefaultDeviceName())))
		outputBuffer.Reset()
	})

	It("sets dns and search domain where dns and search domains have not been explicitly configured", func() {

		nc, err = network.NewNetworkContext()
		Expect(err).NotTo(HaveOccurred())
		dnsManager, err := nc.NewDNSManager()
		Expect(err).NotTo(HaveOccurred())

		// reset dns settings
		err = networksetup.Run([]string{ "-setdnsservers", nc.DefaultDeviceName(), "1.1.1.1" })
		Expect(err).NotTo(HaveOccurred())
		outputBuffer.Reset()
		err = networksetup.Run([]string{ "-setsearchdomains", nc.DefaultDeviceName(), "cloudfare.com" })
		Expect(err).NotTo(HaveOccurred())
		outputBuffer.Reset()

		// set test dns settings
		testDNSSetting(nc, dnsManager)

		// verify changes were restored
		err = networksetup.Run([]string{ "-getdnsservers", nc.DefaultDeviceName() })
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Fields(outputBuffer.String())).To(Equal([]string{ "1.1.1.1" }))
		outputBuffer.Reset()
		err = networksetup.Run([]string{ "-getsearchdomains", nc.DefaultDeviceName() })
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Fields(outputBuffer.String())).To(Equal([]string{ "cloudfare.com" }))
		outputBuffer.Reset()
	})
})
