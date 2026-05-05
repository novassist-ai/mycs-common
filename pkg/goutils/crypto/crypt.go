package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

type Crypt struct {
	gcm cipher.AEAD
}

func NewCrypt(key []byte) (*Crypt, error) {

	var (
		err error

		aesCipher cipher.Block
	)

	crypt := &Crypt{}

	if aesCipher, err = aes.NewCipher(key); err != nil {
		return nil, err
	}
	if crypt.gcm, err = cipher.NewGCM(aesCipher); err != nil {
		return nil, err
	}

	return crypt, nil
}

func (c *Crypt) EncryptB64(plainData string) (string, error) {

	return c.EncryptB64Raw([]byte(plainData))
}

func (c *Crypt) EncryptB64Raw(plainData []byte) (string, error) {

	var (
		err error

		cipherData []byte
	)

	if cipherData, err = c.Encrypt(plainData); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(cipherData), nil
}

func (c *Crypt) Encrypt(plainData []byte) ([]byte, error) {

	var (
		err error
	)

	nonce := make([]byte, c.gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return c.gcm.Seal(nonce, nonce, plainData, nil), nil
}

func (c *Crypt) DecryptB64(cipherDataB64 string) (string, error) {

	var (
		err       error
		plainData []byte
	)
	if plainData, err = c.DecryptB64Raw(cipherDataB64); err != nil {
		return "", err
	}
	return string(plainData), nil
}

func (c *Crypt) DecryptB64Raw(cipherDataB64 string) ([]byte, error) {

	var (
		err error

		cipherData, plainData []byte
	)

	if cipherData, err = base64.StdEncoding.DecodeString(cipherDataB64); err != nil {
		return nil, err
	}
	if plainData, err = c.Decrypt(cipherData); err != nil {
		return nil, err
	}
	return plainData, nil
}

func (c *Crypt) Decrypt(cipherData []byte) ([]byte, error) {

	nonceSize := c.gcm.NonceSize()
	if len(cipherData) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := cipherData[:nonceSize], cipherData[nonceSize:]
	return c.gcm.Open(nil, nonce, ciphertext, nil)
}
