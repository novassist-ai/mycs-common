package rest

import (
	"sync"

	"github.com/novassist/mycs-common/pkg/goutils/crypto"
)

// specification for an authenticatable and
// encryptedable rest api interface used to
// secure requests
type AuthCrypt interface {
	// indicates if crypt instance has been
	// initialized with authenticated keys
	// that are synchronized between the 
	// client and server
	IsAuthenticated() bool
	WaitForAuth() bool

	// a key to be passed along with encrypted
	// auth tokens and used to verify the 
	// authenticity of requests
	AuthTokenKey() string

	// crypt instance used to encrypt and 
	// decrypt tokens and payloads. the
	// cipher can change when auth keys
	// are recycled so the mutex should
	// be used to guard access to it
	Crypt() (*crypto.Crypt, *sync.Mutex)
}
