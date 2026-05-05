package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"io"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
)

// in: len - the length of the key to generate
// out: a byte array of random bytes to use as the key
func RandomKey(len int) ([]byte, error) {

	var (
		err error
	)

	key := make([]byte, len)
	if _, err = io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

// in: passphrase - pass phrase to use to create the key
// in: seed - a seed value used to scramble the returned key hash
// out: a 32 byte key hash of the passphrase
func KeyFromPassphrase(passphrase string, seed int64) []byte {

	logger.TraceMessage(
		"Creating key using passphrase '%s' and seed '%d'.",
		passphrase, seed)

	key := sha256.Sum256([]byte(passphrase))
	if seed != int64(0) {
		// XOR 8 bytes of key with given value to scramble
		// further the key generated from the passphrase to
		// prevent attacks that creating keys using well
		// known passphrases

		xorBytes := make([]byte, 8)
		for i := 0; i < 8; i++ {
			xorBytes[i] = byte(seed >> uint(i*8))
		}
		xorKey := make([]byte, 32)
		for i := 0; i < 32; i++ {
			xorKey[i] = key[i] ^ xorBytes[i%8]
		}
		return xorKey
	}

	return key[0:32]
}

// in: key: the key to reseed
// in: origSeed: the original seed used to seed the hash of the passphrase
// in: newSeed: the new seed to apply
// out: the 32 byte key hash with new seed applied
func ReseedKey(key []byte, origSeed, newSeed int64) []byte {

	xorBytes := make([]byte, 8)

	// restore original key hash
	for i := 0; i < 8; i++ {
		xorBytes[i] = byte(origSeed >> uint(i*8))
	}
	for i := 0; i < 32; i++ {
		key[i] ^= xorBytes[i%8]
	}
	// apply new seed to key hash
	for i := 0; i < 8; i++ {
		xorBytes[i] = byte(newSeed >> uint(i*8))
	}
	for i := 0; i < 32; i++ {
		key[i] ^= xorBytes[i%8]
	}
	return key
}
