package crypto_test

import (
	"github.com/novassist/mycs-common/pkg/goutils/crypto"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ECDH", func() {

	Context("creates ecdh key pair and tests creating a shared secret", func() {

		var (
			err error

			key1 *crypto.ECDHKey
			key2 *crypto.ECDHKey
		)

		It("creates two ecdh key pairs and validates creation of a shared secret", func() {
			key1, err = crypto.NewECDHKey()
			Expect(err).ToNot(HaveOccurred())
			key2, err = crypto.NewECDHKey()
			Expect(err).ToNot(HaveOccurred())
			
			publicKey1, err := key1.PublicKey()
			Expect(err).ToNot(HaveOccurred())
			Expect(len(publicKey1)).To(BeNumerically(">", 0))
			publicKey2, err := key2.PublicKey()
			Expect(err).ToNot(HaveOccurred())
			Expect(len(publicKey2)).To(BeNumerically(">", 0))
			Expect(publicKey1).ToNot(Equal(publicKey2))

			sharedSecret1, err := key1.SharedSecret(publicKey2)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(sharedSecret1)).To(BeNumerically(">", 0))
			sharedSecret2, err := key2.SharedSecret(publicKey1)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(sharedSecret2)).To(BeNumerically(">", 0))
			Expect(sharedSecret1).To(Equal(sharedSecret2))
		})

		It("validate creation of a shared secret using public key encoding method used by nodejs", func() {
			key1, err = crypto.NewECDHKey()
			Expect(err).ToNot(HaveOccurred())
			key2, err = crypto.NewECDHKey()
			Expect(err).ToNot(HaveOccurred())
			
			publicKey1 := key1.PublicKeyForNodeJS()
			Expect(len(publicKey1)).To(BeNumerically(">", 0))
			publicKey2 := key2.PublicKeyForNodeJS()
			Expect(len(publicKey2)).To(BeNumerically(">", 0))
			Expect(publicKey1).ToNot(Equal(publicKey2))

			sharedSecret1, err := key1.SharedSecretFromNodeJS(publicKey2)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(sharedSecret1)).To(BeNumerically(">", 0))
			sharedSecret2, err := key2.SharedSecretFromNodeJS(publicKey1)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(sharedSecret2)).To(BeNumerically(">", 0))
			Expect(sharedSecret1).To(Equal(sharedSecret2))
		})
	})
})
