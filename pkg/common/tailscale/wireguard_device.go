package tailscale

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
	"github.com/tailscale/wireguard-go/device"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const ErrNoDevice = "wireguard not initialized"

func (tsd *TailscaleDaemon) WireguardStatusText() (string, error) {

	var (
		err error

		device *wgtypes.Device
		output strings.Builder
	)

	if device, err = tsd.WireguardDevice(); err != nil {
		return "", err
	}
	printDevice(device, &output)
	for _, p := range device.Peers {
		printPeer(p, &output)
	}
	return output.String(), nil
}

func printDevice(d *wgtypes.Device, out io.Writer) {
	const f = `interface: %s (%s)
  public key: %s
  private key: (hidden)
  listening port: %d

`

	fmt.Fprintf(
		out,
		f,
		d.Name,
		d.Type.String(),
		d.PublicKey.String(),
		d.ListenPort)
}

func printPeer(p wgtypes.Peer, out io.Writer) {
	const f = `peer: %s
  endpoint: %s
  allowed ips: %s
  latest handshake: %s
  transfer: %d B received, %d B sent

`

	fmt.Fprintf(
		out,
		f,
		p.PublicKey.String(),
		// TODO(mdlayher): get right endpoint with getnameinfo.
		p.Endpoint.String(),
		ipsString(p.AllowedIPs),
		p.LastHandshakeTime.String(),
		p.ReceiveBytes,
		p.TransmitBytes,
	)
}

func ipsString(ipns []net.IPNet) string {
	ss := make([]string, 0, len(ipns))
	for _, ipn := range ipns {
		ss = append(ss, ipn.String())
	}

	return strings.Join(ss, ", ")
}

// The WireGuard userspace configuration protocol is described here:
// https://www.wireguard.com/xplatform/#cross-platform-userspace-implementation.

// WireguardDevice gathers device information from a device specified by its path
// and returns a client Device type.
func (tsd *TailscaleDaemon) WireguardDevice() (*wgtypes.Device, error) {

	if tsd.wireguardDevice() != nil {
		reader, writer := io.Pipe()
		go func() {
			defer writer.Close()
			if err := tsd.wgDevice.IpcGetOperation(writer); err != nil {
				logger.ErrorMessage("TailscaleDaemon.getDevice(): Error writing response from wireguard device: %s", err.Error())
			}
		}()	

		// Parse the device from the incoming data stream.
		return parseDevice(reader)
	}
	return nil, fmt.Errorf(ErrNoDevice)
}

func (tsd *TailscaleDaemon) wireguardDevice() *device.Device {
	tsd.mx.Lock()
	defer tsd.mx.Unlock()

	return tsd.wgDevice
}

// parseDevice parses a Device and its Peers from an io.Reader.
func parseDevice(r io.Reader) (*wgtypes.Device, error) {
	var dp deviceParser
	s := bufio.NewScanner(r)
	for s.Scan() {
		b := s.Bytes()
		if len(b) == 0 {
			// Empty line, done parsing.
			break
		}

		// All data is in key=value format.
		kvs := bytes.Split(b, []byte("="))
		if len(kvs) != 2 {
			return nil, fmt.Errorf("wguser: invalid key=value pair: %q", string(b))
		}

		dp.Parse(string(kvs[0]), string(kvs[1]))
	}

	if err := s.Err(); err != nil {
		return nil, err
	}

	return dp.Device()
}

// A deviceParser accumulates information about a Device and its Peers.
type deviceParser struct {
	d   wgtypes.Device
	err error

	parsePeers    bool
	peers         int
	hsSec, hsNano int
}

// Device returns a Device or any errors that were encountered while parsing
// a Device.
func (dp *deviceParser) Device() (*wgtypes.Device, error) {
	if dp.err != nil {
		return nil, dp.err
	}

	// Compute remaining fields of the Device now that all parsing is done.
	dp.d.PublicKey = dp.d.PrivateKey.PublicKey()

	return &dp.d, nil
}

// Parse parses a single key/value pair into fields of a Device.
func (dp *deviceParser) Parse(key, value string) {
	switch key {
	case "errno":
		// 0 indicates success, anything else returns an error number that matches
		// definitions from errno.h.
		if errno := dp.parseInt(value); errno != 0 {
			// TODO(mdlayher): return actual errno on Linux?
			dp.err = os.NewSyscallError("read", fmt.Errorf("wguser: errno=%d", errno))
			return
		}
	case "public_key":
		// We've either found the first peer or the next peer.  Stop parsing
		// Device fields and start parsing Peer fields, including the public
		// key indicated here.
		dp.parsePeers = true
		dp.peers++

		dp.d.Peers = append(dp.d.Peers, wgtypes.Peer{
			PublicKey: dp.parseKey(value),
		})
		return
	}

	// Are we parsing peer fields?
	if dp.parsePeers {
		dp.peerParse(key, value)
		return
	}

	// Device field parsing.
	switch key {
	case "private_key":
		dp.d.PrivateKey = dp.parseKey(value)
	case "listen_port":
		dp.d.ListenPort = dp.parseInt(value)
	case "fwmark":
		dp.d.FirewallMark = dp.parseInt(value)
	}
}

// curPeer returns the current Peer being parsed so its fields can be populated.
func (dp *deviceParser) curPeer() *wgtypes.Peer {
	return &dp.d.Peers[dp.peers-1]
}

// peerParse parses a key/value field into the current Peer.
func (dp *deviceParser) peerParse(key, value string) {
	p := dp.curPeer()
	switch key {
	case "preshared_key":
		p.PresharedKey = dp.parseKey(value)
	case "endpoint":
		// p.Endpoint = dp.parseAddr(value)
	case "last_handshake_time_sec":
		dp.hsSec = dp.parseInt(value)
	case "last_handshake_time_nsec":
		dp.hsNano = dp.parseInt(value)

		// Assume that we've seen both seconds and nanoseconds and populate this
		// field now. However, if both fields were set to 0, assume we have never
		// had a successful handshake with this peer, and return a zero-value
		// time.Time to our callers.
		if dp.hsSec > 0 && dp.hsNano > 0 {
			p.LastHandshakeTime = time.Unix(int64(dp.hsSec), int64(dp.hsNano))
		}
	case "tx_bytes":
		p.TransmitBytes = dp.parseInt64(value)
	case "rx_bytes":
		p.ReceiveBytes = dp.parseInt64(value)
	case "persistent_keepalive_interval":
		p.PersistentKeepaliveInterval = time.Duration(dp.parseInt(value)) * time.Second
	case "allowed_ip":
		cidr := dp.parseCIDR(value)
		if cidr != nil {
			p.AllowedIPs = append(p.AllowedIPs, *cidr)
		}
	case "protocol_version":
		p.ProtocolVersion = dp.parseInt(value)
	}
}

// parseKey parses a Key from a hex string.
func (dp *deviceParser) parseKey(s string) wgtypes.Key {
	if dp.err != nil {
		return wgtypes.Key{}
	}

	b, err := hex.DecodeString(s)
	if err != nil {
		dp.err = err
		return wgtypes.Key{}
	}

	key, err := wgtypes.NewKey(b)
	if err != nil {
		dp.err = err
		return wgtypes.Key{}
	}

	return key
}

// parseInt parses an integer from a string.
func (dp *deviceParser) parseInt(s string) int {
	if dp.err != nil {
		return 0
	}

	v, err := strconv.Atoi(s)
	if err != nil {
		dp.err = err
		return 0
	}

	return v
}

// parseInt64 parses an int64 from a string.
func (dp *deviceParser) parseInt64(s string) int64 {
	if dp.err != nil {
		return 0
	}

	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		dp.err = err
		return 0
	}

	return v
}

// parseAddr parses a UDP address from a string.
// func (dp *deviceParser) parseAddr(s string) *net.UDPAddr {
// 	if dp.err != nil {
// 		return nil
// 	}

// 	addr, err := net.ResolveUDPAddr("udp", s)
// 	if err != nil {
// 		dp.err = err
// 		return nil
// 	}

// 	return addr
// }

// parseInt parses an address CIDR from a string.
func (dp *deviceParser) parseCIDR(s string) *net.IPNet {
	if dp.err != nil {
		return nil
	}

	_, cidr, err := net.ParseCIDR(s)
	if err != nil {
		dp.err = err
		return nil
	}

	return cidr
}
