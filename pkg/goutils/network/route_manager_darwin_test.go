//go:build darwin

package network_test

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/novassist/mycs-common/pkg/goutils/network"
	"github.com/novassist/mycs-common/pkg/goutils/run"
	"github.com/mitchellh/go-homedir"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Route Manager", func() {

	var (
		err error

		outputBuffer bytes.Buffer

		nc network.NetworkContext
	)

	BeforeEach(func() {
		var isAdmin bool
		isAdmin, err = run.IsAdmin()
		Expect(err).NotTo(HaveOccurred())
		if !isAdmin {
			Skip("requires root: sudo -E go test -v ./pkg/goutils/network")
		}

		if err = run.RunAsAdminWithArgs([]string{"/sbin/ifconfig", "feth99", "create"}, &outputBuffer, &outputBuffer); err != nil {
			Fail(fmt.Sprintf("exec \"/sbin/ifconfig feth99 create\" failed: \n\n%s\n", outputBuffer.String()))
		}

		nc, err = network.NewNetworkContext()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		nc.Clear()

		if err = run.RunAsAdminWithArgs([]string{ "/sbin/ifconfig", "feth99", "destroy" }, &outputBuffer, &outputBuffer); err != nil {			
			fmt.Printf("exec \"/sbin/ifconfig feth99 destroy\" failed: \n\n%s\n", outputBuffer.String())
		}
	})

	It("retrieves the default interface", func() {

		routeManager, err := nc.NewRouteManager()
		Expect(err).NotTo(HaveOccurred())
		routableInterface, err := routeManager.GetDefaultInterface()
		Expect(err).NotTo(HaveOccurred())
		Expect(routableInterface).ToNot(BeNil())
	})

	It("creates a new default gateway with routes that bypass it", func() {

		routeManager, err := nc.NewRouteManager()
		Expect(err).NotTo(HaveOccurred())
		err = routeManager.AddExternalRouteToIPs([]string{ "34.204.21.102" })
		Expect(err).NotTo(HaveOccurred())
		routableInterface, err := routeManager.NewRoutableInterface("feth99", "192.168.111.2/32")
		Expect(err).NotTo(HaveOccurred())
		err = routableInterface.MakeDefaultRoute()
		Expect(err).NotTo(HaveOccurred())

		home, _ := homedir.Dir()
		netstat, err := run.NewCLI("/usr/sbin/netstat", home, &outputBuffer, &outputBuffer)
		Expect(err).NotTo(HaveOccurred())
		err = netstat.Run([]string{ "-nrf", "inet" })
		Expect(err).NotTo(HaveOccurred())

		matched, unmatched := outputMatcher(outputBuffer, 
			[]*regexp.Regexp{
				regexp.MustCompile(`^default\s+192.168.111.1\s+UGScg?\s+feth99\s+$`),
				regexp.MustCompile(`^34.204.21.102/32\s+([0-9]+\.?)+\s+UGSc\s+en[0-9]\s+$`),
				regexp.MustCompile(`^192.168.111.1/32\s+\S+\s+\S+\s+feth99\s+\!?$`),
				regexp.MustCompile(`^192.168.111.2/32\s+\S+\s+\S+\s+feth99\s+\!?$`),
			},
		)
		Expect(matched).To(Equal(4))
		Expect(unmatched).To(Equal(0))
	})
})