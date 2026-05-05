package crypto_test

import (
	"crypto/sha256"

	"github.com/novassist/mycs-common/pkg/goutils/crypto"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Key", func() {

	Context("key creation", func() {

		It("creates a random key", func() {

			key, err := crypto.RandomKey(32)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(key)).To(Equal(32))

			i := 0
			for j := range key {
				if key[i] != key[j] {
					i = j
				}
			}
			Expect(i).ToNot(Equal(0))
		})

		It("creates a sha256 key signature for a given passphrase", func() {

			passphrase := "this is a test passphrase"
			hash := sha256.Sum256([]byte(passphrase))

			key := crypto.KeyFromPassphrase(passphrase, 0)
			Expect(len(key)).To(Equal(32))
			Expect(key).To(Equal(hash[0:32]))
		})

		It("creates a seeded sha256 key signature for a given passphrase", func() {

			passphrase := "this is a test passphrase"
			hash := sha256.Sum256([]byte(passphrase))
			for i := 0; i < len(hash); i += 8 {
				for j := 0; j < 8; j++ {
					hash[i+j] ^= 0x0F
				}
			}

			key := crypto.KeyFromPassphrase(passphrase, 0x0F0F0F0F0F0F0F0F)
			Expect(len(key)).To(Equal(32))
			Expect(key).To(Equal(hash[0:32]))
		})

		It("reseeds a sha256 key signature for a given passphrase", func() {

			passphrase := "this is a test passphrase"
			hash := sha256.Sum256([]byte(passphrase))
			for i := 0; i < len(hash); i += 8 {
				for j := 0; j < 8; j++ {
					hash[i+j] ^= 0x7F
				}
			}

			key := crypto.KeyFromPassphrase(passphrase, 0x0F0F0F0F0F0F0F0F)
			Expect(len(key)).To(Equal(32))
			Expect(key).ToNot(Equal(hash[0:32]))

			// reseeding with same seed will restore the original key hash
			key = crypto.ReseedKey(key, 0x0F0F0F0F0F0F0F0F, 0x7F7F7F7F7F7F7F7F)
			Expect(len(key)).To(Equal(32))
			Expect(key).To(Equal(hash[0:32]))
		})
	})
})
