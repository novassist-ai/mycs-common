package crypto

import (
	"fmt"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// creates a key pair for the given vpn type
func CreateVPNKeyPair(vpnType string) (string, string, error) {

	var (
		err error
	)

	switch vpnType {
	case "wireguard":
		var key wgtypes.Key
		if key, err = wgtypes.GeneratePrivateKey(); err != nil {
			return "", "", err
		}
		return key.String(), key.PublicKey().String(), nil
		
	default:
		panic(fmt.Sprintf("I do not know how to generate a key pair for vpn type '%s'.", vpnType))
	}
}
