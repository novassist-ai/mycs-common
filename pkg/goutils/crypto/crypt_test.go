package crypto_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/novassist/mycs-common/pkg/goutils/crypto"
)

var _ = Describe("Crypt", func() {

	var (
		err error
	)

	BeforeEach(func() {
	})

	Context("symmetric encryption and decryption", func() {

		It("encrypts and decrypts using a random key", func() {

			var (
				key,
				encryptedData,
				decryptedData []byte

				crypt *crypto.Crypt
			)

			key, err = crypto.RandomKey(32)
			Expect(err).NotTo(HaveOccurred())

			crypt, err = crypto.NewCrypt(key)
			Expect(err).NotTo(HaveOccurred())

			encryptedData, err = crypt.Encrypt([]byte(plainText))
			Expect(err).NotTo(HaveOccurred())
			Expect(encryptedData).ToNot(BeNil())

			decryptedData, err = crypt.Decrypt(encryptedData)
			Expect(err).NotTo(HaveOccurred())
			Expect(decryptedData).ToNot(BeNil())

			Expect(string(decryptedData)).To(Equal(plainText))
		})

		It("encrypts and decrypts using a pass phrase", func() {

			var (
				encryptedData,
				decryptedData []byte

				crypt *crypto.Crypt
			)

			keyXOR := time.Now().UnixNano()

			crypt, err = crypto.NewCrypt(crypto.KeyFromPassphrase("this is a test", keyXOR))
			Expect(err).NotTo(HaveOccurred())

			encryptedData, err = crypt.Encrypt([]byte(plainText))
			Expect(err).NotTo(HaveOccurred())
			Expect(encryptedData).ToNot(BeNil())

			// discard crypt object used for encryption and create new one for decryption
			crypt, err = crypto.NewCrypt(crypto.KeyFromPassphrase("this is a test", keyXOR))
			Expect(err).NotTo(HaveOccurred())

			decryptedData, err = crypt.Decrypt(encryptedData)
			Expect(err).NotTo(HaveOccurred())
			Expect(decryptedData).ToNot(BeNil())

			Expect(string(decryptedData)).To(Equal(plainText))

			// attempt to decrypt using passphrase without correct xor should fail
			keyXOR = time.Now().UnixNano()

			crypt, err = crypto.NewCrypt(crypto.KeyFromPassphrase("this is a test", keyXOR))
			Expect(err).NotTo(HaveOccurred())

			_, err = crypt.Decrypt(encryptedData)
			Expect(err).To(HaveOccurred())
		})

		It("encrypts and decrypts using strings as input and output", func() {

			var (
				encryptedData,
				decryptedData string

				crypt *crypto.Crypt
			)

			crypt, err = crypto.NewCrypt(crypto.KeyFromPassphrase("this is another test", 0))
			Expect(err).NotTo(HaveOccurred())

			encryptedData, err = crypt.EncryptB64(plainText)
			Expect(err).NotTo(HaveOccurred())
			Expect(encryptedData).ToNot(Equal(""))

			// discard crypt object used for encryption and create new one for decryption
			crypt, err = crypto.NewCrypt(crypto.KeyFromPassphrase("this is another test", 0))
			Expect(err).NotTo(HaveOccurred())

			decryptedData, err = crypt.DecryptB64(encryptedData)
			Expect(err).NotTo(HaveOccurred())
			Expect(decryptedData).ToNot(Equal(""))

			Expect(decryptedData).To(Equal(plainText))
		})
	})
})

const plainText = "hey diddle the cat and the fiddle the cow jumped over the moon"
