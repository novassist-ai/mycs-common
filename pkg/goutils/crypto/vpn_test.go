package crypto_test

import (
	"github.com/novassist/mycs-common/pkg/goutils/crypto"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("VPN Keys", func() {

	It("creates a key pair that can be used by wireguard VPN clients", func() {

		privKey, pubKey, err := crypto.CreateVPNKeyPair("wireguard")
		Expect(err).NotTo(HaveOccurred())
		Expect(privKey).To(MatchRegexp("^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$"))
		Expect(pubKey).To(MatchRegexp("^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$"))
	})
})