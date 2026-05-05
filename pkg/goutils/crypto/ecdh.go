package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math/big"
)

type ECDHKey struct {
	privateKey *ecdsa.PrivateKey
}

// creates a new ECDH key
func NewECDHKey() (*ECDHKey, error) {

	var (
		err error
	)
	key := &ECDHKey{}

	if key.privateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
		return nil, err
	}
	return key, nil
}

// retrieves base64 encoded public key
func (key *ECDHKey) PublicKey() (string, error) {

	var (
		err error
		der []byte
	)
	if der, err = x509.MarshalPKIXPublicKey(&key.privateKey.PublicKey); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

// retrieves a shared secret with another 
// ECDH key using that key's public key
func (key *ECDHKey) SharedSecret(otherPublicKey string) ([]byte, error) {

	var (
		err error
		ok  bool

		der       []byte
		pk        interface{}
		publicKey *ecdsa.PublicKey
	)

	if der, err = base64.StdEncoding.DecodeString(otherPublicKey); err != nil {
		return nil, nil
	}
  if pk, err = x509.ParsePKIXPublicKey(der); err != nil {
		return nil, err
	}
	if publicKey, ok = pk.(*ecdsa.PublicKey); !ok {
		return nil, fmt.Errorf("other public key was not an ECDSA public key")
	}
	
	x, _ := publicKey.Curve.ScalarMult(publicKey.X, publicKey.Y, key.privateKey.D.Bytes())
	return x.Bytes(), nil
}

// retrieves base64 encoded public key that 
// can be used by nodejs crypto library
func (key *ECDHKey) PublicKeyForNodeJS() string {

	publicKey := key.privateKey.PublicKey
	buffer := make([]byte, 65)
	
	// RE: https://stackoverflow.com/questions/53576729/ecdh-nodejs-and-c-sharp-key-exchange
	buffer[0] = 4
	copy(buffer[1:], publicKey.X.Bytes())
	copy(buffer[33:], publicKey.Y.Bytes())

	return base64.StdEncoding.EncodeToString(buffer)
}

// retrieves a shared secret with a public 
// key of an ecdh key created by nodejs 
func (key *ECDHKey) SharedSecretFromNodeJS(otherPublicKey string) ([]byte, error) {

	var (
		err error

		buffer    []byte
		X, Y      big.Int
		publicKey *ecdsa.PublicKey
	)

	// decode and extract ecdh public key
	if buffer, err = base64.StdEncoding.DecodeString(otherPublicKey); err != nil {
		return nil, nil
	}
	X.SetBytes(buffer[1:33])
	Y.SetBytes(buffer[33:])
	publicKey = &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X: &X,
		Y: &Y,
	}

	x, _ := publicKey.Curve.ScalarMult(publicKey.X, publicKey.Y, key.privateKey.D.Bytes())
	return x.Bytes(), nil
}
