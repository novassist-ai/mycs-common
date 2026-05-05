package crypto_test

import (
	"encoding/base64"
	"strings"

	"github.com/novassist/mycs-common/pkg/goutils/crypto"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RSA", func() {

	Context("encrypts and decrypts using rsa public/private keys", func() {

		var (
			err error
	
			rsaPublicKey *crypto.RSAPublicKey
			rsaKey       *crypto.RSAKey

			publickKeyPEM, privateKeyPEM string	
		)

		It("creates a key pair and encrypts and decrypts some data", func() {
			
			plainText := `the quick brown fox jumps over the lazy dog`

			testEncryptDecrypt := func(password []byte) {
				privateKeyPEM, publickKeyPEM, err = crypto.CreateRSAKeyPair(password)
				Expect(err).ToNot(HaveOccurred())
				if (password == nil) {
					Expect(privateKeyPEM).To(HavePrefix("-----BEGIN RSA PRIVATE KEY-----\n"))
				} else {
					Expect(privateKeyPEM).To(HavePrefix("-----BEGIN ENCRYPTED PRIVATE KEY-----\n"))
				}
				Expect(publickKeyPEM).To(HavePrefix( "-----BEGIN PUBLIC KEY-----\n"))
	
				rsaPublicKey, err = crypto.NewPublicKeyFromPEM(publickKeyPEM)
				Expect(err).ToNot(HaveOccurred())
				rsaKey, err = crypto.NewRSAKeyFromPEM(privateKeyPEM, password)
				Expect(err).ToNot(HaveOccurred())
	
				cipherText, err := rsaPublicKey.Encrypt([]byte(plainText))
				Expect(err).ToNot(HaveOccurred())
				Expect(base64.StdEncoding.EncodeToString(cipherText)).ToNot(Equal(base64.StdEncoding.EncodeToString([]byte(plainText))))
	
				decryptedText, err := rsaKey.Decrypt(cipherText)
				Expect(err).ToNot(HaveOccurred())
	
				Expect(string(decryptedText)).To(Equal(plainText))	
			}

			testEncryptDecrypt(nil)
			testEncryptDecrypt([]byte("test password"))
		})

		It("encrypts and decrypts some data using pre-created keys", func() {
			rsaPublicKey, err = crypto.NewPublicKeyFromPEM(testPublicKeyPEM)
			Expect(err).ToNot(HaveOccurred())
			rsaKey, err = crypto.NewRSAKeyFromPEM(testPrivateKeyPEM, []byte("MyCloudSpace Dev Key"))
			Expect(err).ToNot(HaveOccurred())

			cipherText, err := rsaPublicKey.Encrypt([]byte(plainText))
			Expect(err).ToNot(HaveOccurred())

			Expect(base64.StdEncoding.EncodeToString(cipherText)).
				ToNot(Equal(base64.StdEncoding.EncodeToString([]byte(plainText))))

			decryptedText, err := rsaKey.Decrypt(cipherText)
			Expect(err).ToNot(HaveOccurred())

			Expect(string(decryptedText)).To(Equal(plainText))
		})


		It("encrypts and decrypts some data using pre-created keys", func() {
			rsaKey, err = crypto.NewRSAKey()
			Expect(err).ToNot(HaveOccurred())

			rsaPublicKey = rsaKey.PublicKey()
			rsaPublicKey.SetKeyID("test-key-id")

			cipherText, err := rsaPublicKey.EncryptBase64([]byte(plainText))
			Expect(err).ToNot(HaveOccurred())

			cipherTextParts := strings.Split(cipherText, "|")
			Expect(cipherTextParts[0]).To(Equal("test-key-id"))
			Expect(base64.StdEncoding.EncodeToString([]byte(cipherTextParts[1]))).
				ToNot(Equal(base64.StdEncoding.EncodeToString([]byte(plainText))))

			decryptedText, err := rsaKey.DecryptBase64(cipherTextParts[1])
			Expect(err).ToNot(HaveOccurred())

			Expect(string(decryptedText)).To(Equal(plainText))
		})

		It("encrypts and decrypts packed data using AES+RSA", func() {
			rsaKey, err = crypto.NewRSAKey()
			Expect(err).ToNot(HaveOccurred())

			cipherData, err := rsaKey.EncryptPack([]byte(dataToEncryptPack))
			Expect(err).ToNot(HaveOccurred())
			plainData, err := rsaKey.DecryptUnpack(cipherData)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(plainData)).To(Equal(dataToEncryptPack))
		})
	})
})

const testPrivateKeyPEM = `-----BEGIN ENCRYPTED PRIVATE KEY-----
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
-----END ENCRYPTED PRIVATE KEY-----`

const testPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEAxS/abuBjl7ZD7OPe5uYw
wYlNbk41iuauEr9AfZ38NHiCbOu5XXUWwWERyDgsNs7aK/SQQvakjVAyzlsJ5gjq
il+reul0vKjyXBknnl67AV32BrHPCwpb1R3hzUJaQE74Q9kPVCtXuQup/yVv00pB
CSXzDHeTinSCFDmpKul4nEjlgggRo5Mt9ztvT/rpIOutTIAtoyxsonQKmrqfF8vQ
x6gGgWrvJNtC2c2RLmDY6ZaIMWzTBtijRmYzbZyGdWfkDn2tU+mOz9ih482sp3kM
wX4wXJcmZX4SOtdCoUBSvkCBXig1Gq5zagNAYJzvu0QF3p7zw2nS9ky5JtW7hmWp
2H8FTS95HoNFuWkSG5HbfYmh00NOXkn4gSi8qHEPQFPRP4Mqd1y9xbNldNXeNruj
tS19VklyAoVpkCv9LOns1TrDvJjkqefYK07SOIdn7LAzGZKE5kFWcsxl+IbpHAkW
9c6CmFo/aJVzP7806wDdip/6civUBdKHDBHzT7yB97NGWQ9Y9anDpixKGLkW7KM/
lLhseuUfbVhGtkBq8L2xp56Kg/mLRVFdA3iYm55GtWZaP9TmWSDz2veC1V+0EYKU
Vv+1cwseYxLec65LJvbU0w+fMztu89jK/iJdak68pkTSgf/fYokuGrTtFN7Xi1GI
kWDOBLVsmoOLdLML0YxNAWUCAwEAAQ==
-----END PUBLIC KEY-----`

const dataToEncryptPack = `Hey, diddle, diddle, the cat and the fiddle
The cow jumped over the moon
The little dog laughed to see such fun
And the dish ran away with the spoon`