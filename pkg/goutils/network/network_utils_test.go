package network_test

import (
	// "fmt"
	"net"

	"github.com/novassist/mycs-common/pkg/goutils/network"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Wireguard Client", func() {

	It("gets next available network device name", func() {
		
		nextIface, err := network.GetNextAvailabeInterface("utun")		
		Expect(err).NotTo(HaveOccurred())

		ifaces, err := net.Interfaces()
		Expect(err).NotTo(HaveOccurred())
		
		found := false
		for _, i := range ifaces {
			if i.Name == nextIface {
				found = true
				break
			}
		}
		Expect(found).To(BeFalse())
	})

// 	It("determine the configured default gateways", func() {

// 		fmt.Println("\n\n**** ROUTE TABLE INFO ****")
// 		fmt.Printf("\ndefault ipv4 gateway: %s\n", network.Network.DefaultIPv4Route)
// 		fmt.Printf("default ipv6 gateway: %s\n",network. Network.DefaultIPv6Route)
// 		fmt.Println("\nScoped Defaults:")
// 		for _, d := range network.Network.ScopedDefaults {
// 			fmt.Printf("  - %s\n", d)
// 		}
// 		fmt.Println("\nStatic Routes:")
// 		for _, d := range network.Network.StaticRoutes {
// 			fmt.Printf("  - %s\n", d)
// 		}
	
// 		fmt.Println("****************************")
// 		fmt.Println()	
// 	})
})