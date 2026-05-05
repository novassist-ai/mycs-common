//go:build linux

package network_test

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"time"

	"github.com/google/nftables"

	"github.com/novassist/mycs-common/pkg/goutils/network"
	"github.com/novassist/mycs-common/pkg/goutils/run"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const manualValidationPauseSecs = 1

var _ = Describe("Route Manager", func() {

	var (
		err error

		outputBuffer bytes.Buffer

		nc network.NetworkContext
	)

	BeforeEach(func() {
		isAdmin, err := run.IsAdmin()
		Expect(err).ToNot(HaveOccurred())
		if !isAdmin {
			Fail("This test needs to be run with root privileges. i.e. sudo -E go test -v ./...")
		}
	})

	Context("creates routes on a new interface", func() {

		BeforeEach(func() {
			if err = run.RunAsAdminWithArgs([]string{ "/usr/sbin/ip", "link", "add", "wg99", "type", "wireguard" }, &outputBuffer, &outputBuffer); err != nil {
				Fail(fmt.Sprintf("exec \"/usr/sbin/ip link add wg99 type wireguard\" failed: \n\n%s\n", outputBuffer.String()))
			}

			nc, err = network.NewNetworkContext()
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			nc.Clear()

			if err = run.RunAsAdminWithArgs([]string{ "/usr/sbin/ip", "link", "delete", "dev", "wg99" }, &outputBuffer, &outputBuffer); err != nil {
				fmt.Printf("exec \"/usr/sbin/ip link delete dev wg99\" failed: \n\n%s\n", outputBuffer.String())
			}
		})

		It("creates a new default gateway with routes that bypass it", func() {

			routeManager, err := nc.NewRouteManager()
			Expect(err).ToNot(HaveOccurred())
			err = routeManager.AddExternalRouteToIPs([]string{ "34.204.21.102" })
			Expect(err).ToNot(HaveOccurred())
			routableInterface, err := routeManager.NewRoutableInterface("wg99", "192.168.111.2/32")
			Expect(err).ToNot(HaveOccurred())
			err = routableInterface.MakeDefaultRoute()
			Expect(err).ToNot(HaveOccurred())

			outputBuffer.Reset()
			err = run.RunAsAdminWithArgs([]string{ "/usr/sbin/ip", "route", "show" }, &outputBuffer, &outputBuffer)
			Expect(err).ToNot(HaveOccurred())

			fmt.Printf("\n%s\n", outputBuffer.String())

			counter := 0
			scanner := bufio.NewScanner(bytes.NewReader(outputBuffer.Bytes()))

			var matchRoutes = func(line string) {
				matched, _ := regexp.MatchString(`^default via 192\.168\.111\.1 dev wg99 $`, line)
				if matched { counter++; return }
				matched, _ = regexp.MatchString(`^34.204.21.102 via [0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3} dev e[a-z0-9]+ $`, line)
				if matched { counter++; return }
				matched, _ = regexp.MatchString(`^192.168.111.0/24 dev wg99 .* link src 192.168.111.2 $`, line)
				if matched { counter++; return }
			}

			for scanner.Scan() {
				line := scanner.Text()
				matchRoutes(line)
				fmt.Printf("Test route: '%s' <= %d\n", line, counter)
			}
			Expect(counter).To(Equal(3))
		})
	})

	Context("creates routes and manages routes", func() {

		var (
			skipTests bool

			itf2 net.Interface
			itf3 net.Interface
		)

		BeforeEach(func() {
			itfs, err := net.Interfaces()
			Expect(err).ToNot(HaveOccurred())
			if len(itfs) < 4 {
				fmt.Println("To test packet forwarding and nat the test environment needs 2 additional interfaces connected to flat networks without DHCP or a gateway.")

				skipTests = true
				return
			}
			itf2 = itfs[2]
			itf3 = itfs[3]

			if err = run.RunAsAdminWithArgs([]string{ "/usr/sbin/ip", "addr", "flush", itf2.Name }, &outputBuffer, &outputBuffer); err != nil {
				Fail(fmt.Sprintf("exec \"/usr/sbin/ip addr flush dev eth1\" failed: \n\n%s\n", outputBuffer.String()))
			}
			if err = run.RunAsAdminWithArgs([]string{ "/usr/sbin/ip", "addr", "add", "192.168.10.1/24", "dev", itf2.Name }, &outputBuffer, &outputBuffer); err != nil {
				Fail(fmt.Sprintf("exec \"/usr/sbin/ip addr add 192.168.10.1/24 dev eth1\" failed: \n\n%s\n", outputBuffer.String()))
			}
			if err = run.RunAsAdminWithArgs([]string{ "/usr/sbin/ip", "addr", "flush", itf3.Name }, &outputBuffer, &outputBuffer); err != nil {
				Fail(fmt.Sprintf("exec \"/usr/sbin/ip addr flush dev eth2\" failed: \n\n%s\n", outputBuffer.String()))
			}
			if err = run.RunAsAdminWithArgs([]string{ "/usr/sbin/ip", "addr", "add", "192.168.11.1/24", "dev", itf3.Name }, &outputBuffer, &outputBuffer); err != nil {
				Fail(fmt.Sprintf("exec \"/usr/sbin/ip addr add 192.168.11.1/24 dev eth2\" failed: \n\n%s\n", outputBuffer.String()))
			}

			nc, err = network.NewNetworkContext()
			Expect(err).ToNot(HaveOccurred())

			fmt.Printf("\n>> default gateway : %+v\n", network.Network.DefaultIPv4Route)
			for _, r := range network.Network.StaticRoutes {
				fmt.Printf(">> static route : %+v\n", r)
			}
			fmt.Println()
		})

		AfterEach(func() {
			if skipTests {
				return
			}

			nc.Clear()
			if err = run.RunAsAdminWithArgs([]string{ "/usr/sbin/ip", "addr", "flush", itf2.Name }, &outputBuffer, &outputBuffer); err != nil {
				fmt.Printf("exec \"/usr/sbin/ip addr flush dev eth1\" failed: \n\n%s\n", outputBuffer.String())
			}
			if err = run.RunAsAdminWithArgs([]string{ "/usr/sbin/ip", "addr", "flush", itf3.Name }, &outputBuffer, &outputBuffer); err != nil {
				fmt.Printf("exec \"/usr/sbin/ip addr flush dev eth2\" failed: \n\n%s\n", outputBuffer.String())
			}
		})

		It("creates routes between interfaces and NATs out", func() {
			if skipTests {
				fmt.Println("No second interface so skipping test \"creates routes between interfaces and NATs out\"...")
			}

			routeManager, err := nc.NewRouteManager()
			Expect(err).ToNot(HaveOccurred())
			_, err = routeManager.NewFilterRouter(true)
			Expect(err).ToNot(HaveOccurred())

			ritf1, err := routeManager.GetDefaultInterface()            // interface to world
			Expect(err).ToNot(HaveOccurred())
			ritf2, err := routeManager.GetRoutableInterface(itf2.Name)  // interface to lan1
			Expect(err).ToNot(HaveOccurred())
			ritf3, err := routeManager.GetRoutableInterface(itf3.Name)  // interface to lan2
			Expect(err).ToNot(HaveOccurred())

			ritf2IP, ritf2NW, err := ritf2.Address4()
			Expect(err).ToNot(HaveOccurred())
			Expect(ritf2IP).To(Equal("192.168.10.1"))
			Expect(ritf2NW).To(Equal("192.168.10.0/24"))
			ritf3IP, ritf3NW, err := ritf3.Address4()
			Expect(err).ToNot(HaveOccurred())
			Expect(ritf3IP).To(Equal("192.168.11.1"))
			Expect(ritf3NW).To(Equal("192.168.11.0/24"))

			// forward packets from lan1 to world (ip v4)
			_, err = ritf1.FowardTrafficFrom(ritf2, network.LAN4, network.WORLD4, true)
			Expect(err).ToNot(HaveOccurred())
			// forward packets from lan1 to lan2 (ip v4)
			_, err = ritf3.FowardTrafficFrom(ritf2, network.LAN4, network.LAN4, false)
			Expect(err).ToNot(HaveOccurred())
			// forward packets from lan2 to external network 8.8.8.8/32 only (ip v4)
			_, err = ritf1.FowardTrafficFrom(ritf3, network.LAN4, "8.8.8.8/32", true)
			Expect(err).ToNot(HaveOccurred())
			// forward packets from lan2 to lan1 (ip v4)
			_, err = ritf2.FowardTrafficFrom(ritf3, network.LAN4, network.LAN4, false)
			Expect(err).ToNot(HaveOccurred())

			showNftRuleset()

			forwardRuleMatches := []*regexp.Regexp{
				regexp.MustCompile(`^\s+ct state vmap @ctstate\s*$`),
				// routing between lan1 to lan2
				regexp.MustCompile(`^\s+iifname "eth1" oifname "eth2" ip saddr 192.168.10.0/24 ip daddr 192.168.11.0/24 accept\s*$`),
				regexp.MustCompile(`^\s+iifname "eth2" oifname "eth1" ip saddr 192.168.11.0/24 ip daddr 192.168.10.0/24 accept\s*$`),
				// allow lan1 access to internet
				regexp.MustCompile(`^\s+iifname "eth1" oifname "eth0" ip saddr 192.168.10.0/24 accept\s*$`),
				// allow lan2 access to only 8.8.8.8 externally
				regexp.MustCompile(`^\s+iifname "eth2" oifname "eth0" ip saddr 192.168.11.0/24 ip daddr 8.8.8.8 accept\s*$`),
			}
			natPostRuleMatches := []*regexp.Regexp{
				// masq lan1 to world
				regexp.MustCompile(`^\s+oifname "eth0" ip saddr 192.168.10.0/24 masquerade\s*$`),
				// masq lan2 to only 8.8.8.8 externally
				regexp.MustCompile(`^\s+oifname "eth0" ip saddr 192.168.11.0/24 ip daddr 8.8.8.8 masquerade\s*$`),
			}

			testAppliedConfig("forward chain rules after config",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain forward {/,/}/p'",
				forwardRuleMatches, 5, 0,
			)
			testAppliedConfig("nat post-routing chain rules after config",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain nat_postrouting {/,/}/p'",
				natPostRuleMatches, 2, 0,
			)
			time.Sleep(time.Second * manualValidationPauseSecs) // increase to pause for manual validation

			// delete forwarding rules
			err = ritf1.DeleteTrafficForwardedFrom(ritf3, network.LAN4, "8.8.8.8/32")
			Expect(err).ToNot(HaveOccurred())

			testAppliedConfig("forward chain rules after delete",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain forward {/,/}/p'",
				forwardRuleMatches, 4, 1,
			)
			testAppliedConfig("nat post-routing chain rules after delete",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain nat_postrouting {/,/}/p'",
				natPostRuleMatches, 1, 1,
			)
			time.Sleep(time.Second * manualValidationPauseSecs) // increase to pause for manual validation
		})

		It("forwards a port to another host", func() {
			if skipTests {
				fmt.Println("No second interface so skipping test \"forwards a port to another host\"...")
			}

			routeManager, err := nc.NewRouteManager()
			Expect(err).ToNot(HaveOccurred())

			filterRouter, err := routeManager.NewFilterRouter(false)
			Expect(err).ToNot(HaveOccurred())

			ritf2, err := routeManager.GetRoutableInterface(itf2.Name)  // interface to lan1
			Expect(err).ToNot(HaveOccurred())

			// forward 192.168.10.1:8080 to 192.168.11.1:80
			_, err = ritf2.ForwardPortTo(network.TCP, 8080, 80, netip.MustParseAddr("192.168.11.10"))
			Expect(err).ToNot(HaveOccurred())
			// forward :8888 to 192.168.10.1:80
			_, err = filterRouter.ForwardPort(8888, 80, netip.MustParseAddr("192.168.10.10"), network.TCP)
			Expect(err).ToNot(HaveOccurred())

			showNftRuleset()

			forwardRuleMatches := []*regexp.Regexp{
				regexp.MustCompile(`^\s+ct state vmap @ctstate\s*$`),
				// allow port forward from 192.168.10.1:8080 to 192.168.11.10:80
				regexp.MustCompile(`^\s+ip daddr 192.168.11.10 accept\s*$`),
				// allow port forward from :8888 to 192.168.10.10:80
				regexp.MustCompile(`^\s+ip daddr 192.168.10.10 accept\s*$`),
			}
			natPreRuleMatches := []*regexp.Regexp{
				// forward 192.168.10.1:8080 to 192.168.11.1:80
				regexp.MustCompile(`^\s+ip daddr 192.168.10.1 tcp dport 8080 dnat to 192.168.11.10:80\s*$`),
				// forward incoming requests to 8888 on all interfaces to 192.168.10.10:80
				regexp.MustCompile(`^\s+tcp dport 8888 dnat to 192.168.10.10:80\s*$`),
			}
			natPostRuleMatches := []*regexp.Regexp{
				// masq traffic forwarded from 192.168.11.10:8080 to 192.168.11.1:80
				regexp.MustCompile(`^\s+ip daddr 192.168.11.10 masquerade\s*$`),
				// masq traffic forwarded to 192.168.10.10:80
				regexp.MustCompile(`^\s+ip daddr 192.168.10.10 masquerade\s*$`),
			}

			testAppliedConfig("forward chain rules after config",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain forward {/,/}/p'",
				forwardRuleMatches, 3, 0,
			)
			testAppliedConfig("nat pre-routing chain rules after config",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain nat_prerouting {/,/}/p'",
				natPreRuleMatches, 2, 0,
			)
			testAppliedConfig("nat post-routing chain rules after config",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain nat_postrouting {/,/}/p'",
				natPostRuleMatches, 2, 0,
			)
			time.Sleep(time.Second * manualValidationPauseSecs) // increase to pause for manual validation

			// delete port forwarding rules
			err = ritf2.DeletePortForwardedTo(network.TCP, 8080, 80, netip.MustParseAddr("192.168.11.10"))
			Expect(err).ToNot(HaveOccurred())

			testAppliedConfig("forward chain rules after delete",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain forward {/,/}/p'",
				forwardRuleMatches, 2, 1,
			)
			testAppliedConfig("nat pre-routing chain rules after delete",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain nat_prerouting {/,/}/p'",
				natPreRuleMatches, 1, 1,
			)
			testAppliedConfig("nat post-routing chain rules after delete",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain nat_postrouting {/,/}/p'",
				natPostRuleMatches, 1, 1,
			)
			time.Sleep(time.Second * manualValidationPauseSecs) // increase to pause for manual validation
		})

		FIt("applies firewall rules using security groups", func() {
			if skipTests {
				fmt.Println("No second interface so skipping test \"applies firewall rules using security groups\"...")
			}

			routeManager, err := nc.NewRouteManager()
			Expect(err).ToNot(HaveOccurred())
			filterRouter, err := routeManager.NewFilterRouter(true)
			Expect(err).ToNot(HaveOccurred())

			ritf2, err := routeManager.GetRoutableInterface(itf2.Name)  // interface to lan1
			Expect(err).ToNot(HaveOccurred())
			ritf3, err := routeManager.GetRoutableInterface(itf3.Name)  // interface to lan2
			Expect(err).ToNot(HaveOccurred())

			// security group allow ssh from any interface
			allowSSH := network.SecurityGroup{
				Ports: []network.PortGroup{
					{
						Proto: network.TCP,
						FromPort: 22,
						ToPort: 22,
					},
				},
			}
			err = filterRouter.SetSecurityGroups([]network.SecurityGroup{allowSSH}, "")
			Expect(err).ToNot(HaveOccurred())
			allowCustom := network.SecurityGroup{
				Ports: []network.PortGroup{
					{
						Proto: network.TCP,
						FromPort: 34080,
						ToPort: 34080,
					},
					{
						Proto: network.TCP,
						FromPort: 34443,
						ToPort: 34443,
					},
					{
						Proto: network.UDP,
						FromPort: 35555,
						ToPort: 35555,
					},
				},
			}
			err = filterRouter.SetSecurityGroups([]network.SecurityGroup{allowCustom}, "")
			Expect(err).ToNot(HaveOccurred())
			denyMultipleTo11 := network.SecurityGroup{
				Deny: true,
				SrcNetwork: netip.MustParsePrefix("192.168.10.10/24"),
				DstNetwork: netip.MustParsePrefix("192.168.11.10/24"),
				Ports: []network.PortGroup{
					{
						Proto: network.TCP,
						FromPort: 80,
						ToPort: 80,
					},
					{
						Proto: network.TCP,
						FromPort: 22,
						ToPort: 22,
					},
					{
						Proto: network.ICMP,
					},
				},
			}
			allowICMToItf2 := network.SecurityGroup{
				Ports: []network.PortGroup{
					{
						Proto: network.ICMP,
					},
				},
			}
			err = ritf2.SetSecurityGroups([]network.SecurityGroup{allowICMToItf2,denyMultipleTo11})
			Expect(err).ToNot(HaveOccurred())
			denyHTTPto10 := network.SecurityGroup{
				Deny: true,
				DstNetwork: netip.MustParsePrefix("192.168.10.10/24"),
				Ports: []network.PortGroup{
					{
						Proto: network.TCP,
						FromPort: 80,
						ToPort: 80,
					},
				},
			}
			allowICMToItf3 := network.SecurityGroup{
				Ports: []network.PortGroup{
					{
						Proto: network.ICMP,
					},
				},
			}
			err = ritf3.SetSecurityGroups([]network.SecurityGroup{allowICMToItf3,denyHTTPto10})
			Expect(err).ToNot(HaveOccurred())

			// forward packets from lan1 to lan2 (ip v4)
			_, err = ritf3.FowardTrafficFrom(ritf2, network.LAN4, network.LAN4, false)
			Expect(err).ToNot(HaveOccurred())
			// forward packets from lan2 to lan1 (ip v4)
			_, err = ritf2.FowardTrafficFrom(ritf3, network.LAN4, network.LAN4, false)
			Expect(err).ToNot(HaveOccurred())

			showNftRuleset()

			_, vmNameForAllowSSHip4, _ := allowSSH.CreateSecurityGroupKeys("")
			_, vmNameForAllowCustomip4, _ := allowCustom.CreateSecurityGroupKeys("")
			_, vmNameForDenyMultipleTo11, _ := denyMultipleTo11.CreateSecurityGroupKeys(ritf2.Name())
			_, vmNameForDenyHTTPto10, _ := denyHTTPto10.CreateSecurityGroupKeys(ritf3.Name())

			inputRuleMatches := []*regexp.Regexp{
				regexp.MustCompile(`^\s+type filter hook input priority filter; policy drop;\s*$`),
				regexp.MustCompile(`^\s+ct state vmap @ctstate\s*$`),
				regexp.MustCompile(`^\s+iifname vmap @inbound_ifname\s*$`),
				regexp.MustCompile(`^\s+ip protocol . th dport vmap @`+vmNameForAllowSSHip4[0]+`\s*$`),
			}
			forwardRuleMatches := []*regexp.Regexp{
				regexp.MustCompile(`^\s+type filter hook forward priority filter; policy drop;\s*$`),
				regexp.MustCompile(`^\s+ct state vmap @ctstate\s*$`),
				regexp.MustCompile(`^\s+iifname "eth1" ip saddr 192.168.10.0/24 ip daddr 192.168.11.0/24 ip protocol . th dport vmap @`+vmNameForDenyMultipleTo11[0]+`\s*$`),
				regexp.MustCompile(`^\s+iifname "eth1" ip saddr 192.168.10.0/24 ip daddr 192.168.11.0/24 meta l4proto icmp drop\s*$`),
				regexp.MustCompile(`^\s+iifname "eth2" ip daddr 192.168.10.0/24 ip protocol . th dport vmap @`+vmNameForDenyHTTPto10[0]+`\s*$`),
				// routing between lan1 to lan2
				regexp.MustCompile(`^\s+iifname "eth1" oifname "eth2" ip saddr 192.168.10.0/24 ip daddr 192.168.11.0/24 accept\s*$`),
				regexp.MustCompile(`^\s+iifname "eth2" oifname "eth1" ip saddr 192.168.11.0/24 ip daddr 192.168.10.0/24 accept\s*$`),
			}
			inboundItf2Matches := []*regexp.Regexp{
				regexp.MustCompile(`^\s+meta l4proto icmp accept\s*$`),
			}
			inboundItf3Matches := []*regexp.Regexp{
				regexp.MustCompile(`^\s+meta l4proto icmp accept\s*$`),
			}

			testAppliedConfig("input chain rules after config",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain input {/,/}/p'",
				inputRuleMatches, 4, 0,
			)
			testAppliedConfig("forward chain rules after config",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain forward {/,/}/p'",
				forwardRuleMatches, 7, 0,
			)
			testAppliedConfig("inbound eth1 chain rules after config",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain inbound_eth1 {/,/}/p'",
				inboundItf2Matches, 1, 0,
			)
			testAppliedConfig("inbound eth2 chain rules after config",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain inbound_eth2 {/,/}/p'",
				inboundItf3Matches, 1, 0,
			)
			testPorts(vmNameForAllowSSHip4[0], allowSSH, true)
			testPorts(vmNameForAllowCustomip4[0], allowCustom, true)
			testPorts(vmNameForDenyMultipleTo11[0], denyMultipleTo11, true)
			testPorts(vmNameForDenyHTTPto10[0], denyHTTPto10, true)

			time.Sleep(time.Second * manualValidationPauseSecs) // increase to pause for manual validation

			// delete a security group that has overlapping rules
			err = filterRouter.DeleteSecurityGroups([]network.SecurityGroup{allowCustom}, "")
			Expect(err).ToNot(HaveOccurred())

			testPorts(vmNameForAllowSSHip4[0], allowSSH, true)
			testPorts(vmNameForAllowCustomip4[0], allowCustom, false)

			testAppliedConfig("input chain rules after delete",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain input {/,/}/p'",
				inputRuleMatches, 4, 0,
			)

			// completely delete all overlapping rules
			err = filterRouter.DeleteSecurityGroups([]network.SecurityGroup{allowSSH}, "")
			Expect(err).ToNot(HaveOccurred())

			testPorts(vmNameForAllowSSHip4[0], allowSSH, false)
			testPorts(vmNameForAllowCustomip4[0], allowCustom, false)

			testAppliedConfig("input chain rules after delete",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain input {/,/}/p'",
				inputRuleMatches, 3, 1,
			)

			// delete another couple of rules
			err = ritf2.DeleteSecurityGroups([]network.SecurityGroup{allowICMToItf2,denyMultipleTo11})
			Expect(err).ToNot(HaveOccurred())

			testPorts(vmNameForDenyMultipleTo11[0], denyMultipleTo11, false)

			testAppliedConfig("forward chain rules after config",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain forward {/,/}/p'",
				forwardRuleMatches, 5, 2,
			)
			testAppliedConfig("inbound eth1 chain rules after config",
				"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/chain inbound_eth1 {/,/}/p'",
				inboundItf2Matches, 0, 1,
			)

			// add back the rule and validate no errors
			err = filterRouter.SetSecurityGroups([]network.SecurityGroup{allowCustom}, "")
			Expect(err).ToNot(HaveOccurred())
			err = filterRouter.SetSecurityGroups([]network.SecurityGroup{allowSSH}, "")
			Expect(err).ToNot(HaveOccurred())
			err = ritf2.SetSecurityGroups([]network.SecurityGroup{allowICMToItf2,denyMultipleTo11})
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(time.Second * manualValidationPauseSecs) // increase to pause for manual validation
		})

		It("creates a deny list by ip address", func() {
			if skipTests {
				fmt.Println("No second interface so skipping test \"creates a deny list by ip address\"...")
			}

			routeManager, err := nc.NewRouteManager()
			Expect(err).ToNot(HaveOccurred())
			filterRouter, err := routeManager.NewFilterRouter(false)
			Expect(err).ToNot(HaveOccurred())

			// time.Sleep(time.Second * 5)

			denyList := []netip.Addr{
				netip.MustParseAddr("192.168.11.10"),
				netip.MustParseAddr("192.168.11.11"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::10"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::11"),
				netip.MustParseAddr("192.168.11.12"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::100"),
				netip.MustParseAddr("192.168.11.13"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::101"),
			}
			err = filterRouter.AddIPsToDenyList(denyList)
			Expect(err).ToNot(HaveOccurred())

			showNftRuleset()
			testIPSetElements("ip_denylist", denyList)

			time.Sleep(time.Second * manualValidationPauseSecs) // increase to pause for manual validation

			err = filterRouter.DeleteIPsFromDenyList([]netip.Addr{
				netip.MustParseAddr("192.168.11.10"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::11"),
				netip.MustParseAddr("192.168.11.12"),
				netip.MustParseAddr("192.168.11.13"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::101"),
			})
			Expect(err).ToNot(HaveOccurred())

			showNftRuleset()
			testIPSetElements("ip_denylist", []netip.Addr{
				netip.MustParseAddr("192.168.11.11"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::10"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::100"),
			})

			err = filterRouter.DeleteIPsFromDenyList([]netip.Addr{
				netip.MustParseAddr("192.168.11.11"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::10"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::100"),
			})
			Expect(err).ToNot(HaveOccurred())

			showNftRuleset()
			testIPSetElements("ip_denylist", []netip.Addr{})
		})

		It("creates a allow list by ip address", func() {
			if skipTests {
				fmt.Println("No second interface so skipping test \"creates an allow list by ip address\"...")
			}

			routeManager, err := nc.NewRouteManager()
			Expect(err).ToNot(HaveOccurred())
			filterRouter, err := routeManager.NewFilterRouter(false)
			Expect(err).ToNot(HaveOccurred())

			// time.Sleep(time.Second * 5)

			allowList := []netip.Addr{
				netip.MustParseAddr("192.168.100.2"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::10"),
				netip.MustParseAddr("192.168.11.10"),
				netip.MustParseAddr("192.168.10.11"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::11"),
				netip.MustParseAddr("192.168.11.12"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::100"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::101"),
			}
			err = filterRouter.AddIPsToAllowList(allowList)
			Expect(err).ToNot(HaveOccurred())

			showNftRuleset()
			testIPSetElements("ip_allowlist", allowList)

			time.Sleep(time.Second * manualValidationPauseSecs) // increase to pause for manual validation

			err = filterRouter.DeleteIPsFromAllowList([]netip.Addr{
				netip.MustParseAddr("192.168.11.10"),
				netip.MustParseAddr("192.168.10.11"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::100"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::101"),
			})
			Expect(err).ToNot(HaveOccurred())
			remainingIPs := []netip.Addr{
				netip.MustParseAddr("192.168.100.2"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::10"),
				netip.MustParseAddr("fd36:a851:bdf7:078d::11"),
				netip.MustParseAddr("192.168.11.12"),
			}

			showNftRuleset()
			testIPSetElements("ip_allowlist", remainingIPs)

			err = filterRouter.DeleteIPsFromAllowList(remainingIPs)
			Expect(err).ToNot(HaveOccurred())

			showNftRuleset()
			testIPSetElements("ip_allowlist", []netip.Addr{})
		})
	})
})

func showNftRuleset() {

	var (
		err error

		outputBuffer bytes.Buffer
	)

	err = run.RunAsAdminWithArgs([]string{ "/usr/sbin/ip", "route", "show" }, &outputBuffer, &outputBuffer)
	Expect(err).ToNot(HaveOccurred())
	fmt.Printf("\n# ip route show\n=====\n%s=====\n", outputBuffer.String())

	outputBuffer.Reset()
	err = run.RunAsAdminWithArgs([]string{
		"/bin/sh", "-c",
		"nft list ruleset",
	}, &outputBuffer, &outputBuffer)
	Expect(err).ToNot(HaveOccurred())
	fmt.Printf("\n# nft list ruleset\n=====\n%s=====\n\n", outputBuffer.String())
}

func testPorts(vmapName string, sg network.SecurityGroup, isSet bool) {

	verdict := "accept"
	if sg.Deny {
		verdict = "drop"
	}

	portMatches := []*regexp.Regexp{}
	numMatches := 0
	for _, pg := range sg.Ports {
		if pg.Proto != network.ICMP {
			for p := pg.FromPort; p <= pg.ToPort; p++ {
				portMatches = append(portMatches, regexp.MustCompile(fmt.Sprintf("%s . %d : %s", pg.Proto, p, verdict)))
				numMatches++
			}
		}
	}
	if isSet {
		testAppliedConfig(fmt.Sprintf("values present test for vmap %s", vmapName),
			"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/map "+vmapName+" {/,/}/p'",
			portMatches, numMatches, 0,
		)
	} else {
		testAppliedConfig(fmt.Sprintf("values not present test for vmap %s", vmapName),
			"nft list ruleset | sed -n '/^table ip mycs_router_ipv4 {/,/^}/p' | sed -n '/map "+vmapName+" {/,/}/p'",
			portMatches, 0, numMatches,
		)		
	}
}

func testIPSetElements(setName string, ips []netip.Addr) {

	nfc, err := nftables.New(nftables.AsLasting())
	Expect(err).ToNot(HaveOccurred())

	ipsByType := make([][]netip.Addr, 2)
	for _, ip := range ips {
		if ip.Is4() {
			ipsByType[0] = append(ipsByType[0], ip)
		} else {
			ipsByType[1] = append(ipsByType[1], ip)
		}
	}

	tables := getMycsNftTables(nfc)
	for i, table := range tables {
		set := getMycsNftSets(nfc, table, setName)
		elems, err := nfc.GetSetElements(set)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(elems)).To(Equal(len(ipsByType[i])))
		for _, elem := range elems {						
			for j, ip := range ipsByType[i] {
				if bytes.Equal(elem.Key, ip.AsSlice()) {
					// remove found element
					ipsByType[i] = append(ipsByType[i][:j], ipsByType[i][j+1:]...)
				}
			}
		}
		Expect(len(ipsByType[i])).To(Equal(0))
	}
}

func getMycsNftTables(nfc *nftables.Conn) []*nftables.Table {

	mycsTables := make([]*nftables.Table, 2)

	tables, err := nfc.ListTablesOfFamily(nftables.TableFamilyIPv4)
	Expect(err).ToNot(HaveOccurred())
	for _, table := range tables {
		if table.Name == `mycs_router_ipv4` { 
			mycsTables[0] = table
			break
	 	}
	}
	Expect(mycsTables[0]).ToNot(BeNil())

	tables, err = nfc.ListTablesOfFamily(nftables.TableFamilyIPv6)
	Expect(err).ToNot(HaveOccurred())
	for _, table := range tables {
		if table.Name == `mycs_router_ipv6` { 
			mycsTables[1] = table
			break
		}
	}
	Expect(mycsTables[1]).ToNot(BeNil())

	return mycsTables
}

func getMycsNftSets(nfc *nftables.Conn, table *nftables.Table, setName string) (mycsSet *nftables.Set) {

	sets, err := nfc.GetSets(table)
	Expect(err).ToNot(HaveOccurred())
	for _, set := range sets {
		if set.Name == setName { 
			mycsSet = set
			break
		}
	}
	Expect(mycsSet).ToNot(BeNil())
	return mycsSet
}
