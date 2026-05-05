package mocks

import (
	"crypto/rand"
	"io"
	"sync"

	"github.com/novassist/mycs-common/pkg/goutils/crypto"
)

type MockAuthCrypt struct {
	c *crypto.Crypt
	l sync.Mutex

	k string
}

func NewMockAuthCrypt(key string, encryptionKey []byte) (*MockAuthCrypt, error) {

	var (
		err error
	)

	restApiAuth := &MockAuthCrypt{
		k: key,
	}
	if (encryptionKey == nil) {
		encryptionKey = make([]byte, 32)
		if _, err = io.ReadFull(rand.Reader, encryptionKey); err != nil {
			return nil, err
		}	
	}
	if restApiAuth.c, err = crypto.NewCrypt(encryptionKey); err != nil {
		return nil, err
	}
	return restApiAuth, nil
}

func (a *MockAuthCrypt) WaitForAuth() bool {
	return true
}

func (a *MockAuthCrypt) IsAuthenticated() bool {
	return true
}

func (a *MockAuthCrypt) Crypt() (*crypto.Crypt, *sync.Mutex) {
	return a.c, &a.l
}

func (a *MockAuthCrypt) AuthTokenKey() string {
	return a.k
}
