package lgdeck

import (
	"fmt"
	"net"
	"strings"
)

const (
	wolSyncByte       = 0xff
	wolSyncCount      = 6
	wolMacRepeat      = 16
	wolMacBlocksCount = 6
	wolBroadcast      = "255.255.255.255"
	wolPort           = 9
)

// WakeOnLan sends a magic packet to the given MAC address via UDP broadcast.
func WakeOnLan(macAddress string) error {
	macAddress = strings.ToLower(strings.TrimSpace(macAddress))
	blocks := strings.Split(macAddress, ":")
	if len(blocks) != wolMacBlocksCount {
		return fmt.Errorf("invalid MAC address: %q", macAddress)
	}

	macBytes := make([]byte, wolMacBlocksCount)
	for i, b := range blocks {
		var val byte
		_, err := fmt.Sscanf(b, "%02x", &val)
		if err != nil {
			return fmt.Errorf("invalid MAC byte %q: %w", b, err)
		}
		macBytes[i] = val
	}

	packetLen := wolSyncCount + wolMacBlocksCount*wolMacRepeat
	packet := make([]byte, packetLen)
	for i := 0; i < wolSyncCount; i++ {
		packet[i] = wolSyncByte
	}
	for i := 0; i < wolMacRepeat; i++ {
		for j := 0; j < wolMacBlocksCount; j++ {
			packet[wolSyncCount+i*wolMacBlocksCount+j] = macBytes[j]
		}
	}

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", wolBroadcast, wolPort))
	if err != nil {
		return fmt.Errorf("resolving WoL address: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("dial UDP for WoL: %w", err)
	}
	defer conn.Close()

	_, err = conn.Write(packet)
	return err
}
