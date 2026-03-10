// kernel_l2tp registers L2TP tunnel/session with the Linux kernel via netlink
// so the PPPoL2TP socket can connect to the same UDP FD.
package client

import (
	"fmt"
	"log"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
)

// L2TP netlink constants (match linux/include/uapi/linux/l2tp.h)
const (
	l2tpGenlName    = "l2tp"
	l2tpGenlVersion = 1

	l2tpCmdTunnelCreate  = 1
	l2tpCmdTunnelDelete  = 2
	l2tpCmdSessionCreate = 5
	l2tpCmdSessionDelete = 6

	l2tpAttrConnId        = 9
	l2tpAttrPeerConnId    = 10
	l2tpAttrSessionId     = 11
	l2tpAttrPeerSessionId = 12
	l2tpAttrProtoVersion  = 7
	l2tpAttrEncapType     = 2
	l2tpAttrFd            = 23
	l2tpAttrPwType        = 1
	l2tpAttrL2specType    = 5
	l2tpAttrL2specLen     = 6
	l2tpAttrDebug         = 17

	l2tpEncapUDP     = 0
	l2tpPwTypePPP    = 0x0007
	l2tpL2specNone   = 0
	l2tpProtoVersion2 = 2
)

// kernelL2TP holds a netlink connection to the kernel L2TP subsystem for
// registering tunnel/session so PPPoL2TP connect() can use the same FD.
type kernelL2TP struct {
	conn     *genetlink.Conn
	familyID uint16
}

// newKernelL2TP connects to the kernel L2TP netlink family.
func newKernelL2TP() (*kernelL2TP, error) {
	conn, err := genetlink.Dial(nil)
	if err != nil {
		return nil, fmt.Errorf("genetlink dial: %w", err)
	}
	family, err := conn.GetFamily(l2tpGenlName)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("get l2tp family: %w", err)
	}
	return &kernelL2TP{conn: conn, familyID: family.ID}, nil
}

// RegisterTunnel creates the L2TP tunnel in the kernel with the given FD and IDs.
// Must be called after userspace L2TP control plane has established the tunnel.
func (k *kernelL2TP) RegisterTunnel(fd int, tunnelID, peerTunnelID uint32) error {
	attrs := []netlink.Attribute{
		{Type: l2tpAttrConnId, Data: nlenc.Uint32Bytes(tunnelID)},
		{Type: l2tpAttrPeerConnId, Data: nlenc.Uint32Bytes(peerTunnelID)},
		{Type: l2tpAttrProtoVersion, Data: nlenc.Uint8Bytes(l2tpProtoVersion2)},
		{Type: l2tpAttrEncapType, Data: nlenc.Uint16Bytes(l2tpEncapUDP)},
		{Type: l2tpAttrFd, Data: nlenc.Uint32Bytes(uint32(fd))},
		{Type: l2tpAttrDebug, Data: nlenc.Uint32Bytes(0)},
	}
	data, err := netlink.MarshalAttributes(attrs)
	if err != nil {
		return err
	}
	req := genetlink.Message{
		Header: genetlink.Header{Command: l2tpCmdTunnelCreate, Version: l2tpGenlVersion},
		Data:   data,
	}
	_, err = k.conn.Execute(req, k.familyID, netlink.Request|netlink.Acknowledge)
	if err != nil {
		return fmt.Errorf("L2TP_CMD_TUNNEL_CREATE: %w", err)
	}
	log.Printf("Kernel L2TP tunnel registered: tid=%d ptid=%d fd=%d", tunnelID, peerTunnelID, fd)
	return nil
}

// RegisterSession creates the L2TP session in the kernel. Tunnel must exist.
func (k *kernelL2TP) RegisterSession(tunnelID, peerTunnelID, sessionID, peerSessionID uint32) error {
	attrs := []netlink.Attribute{
		{Type: l2tpAttrConnId, Data: nlenc.Uint32Bytes(tunnelID)},
		{Type: l2tpAttrPeerConnId, Data: nlenc.Uint32Bytes(peerTunnelID)},
		{Type: l2tpAttrSessionId, Data: nlenc.Uint32Bytes(sessionID)},
		{Type: l2tpAttrPeerSessionId, Data: nlenc.Uint32Bytes(peerSessionID)},
		{Type: l2tpAttrPwType, Data: nlenc.Uint16Bytes(l2tpPwTypePPP)},
		{Type: l2tpAttrL2specType, Data: nlenc.Uint8Bytes(l2tpL2specNone)},
		{Type: l2tpAttrL2specLen, Data: nlenc.Uint8Bytes(0)},
	}
	data, err := netlink.MarshalAttributes(attrs)
	if err != nil {
		return err
	}
	req := genetlink.Message{
		Header: genetlink.Header{Command: l2tpCmdSessionCreate, Version: l2tpGenlVersion},
		Data:   data,
	}
	_, err = k.conn.Execute(req, k.familyID, netlink.Request|netlink.Acknowledge)
	if err != nil {
		return fmt.Errorf("L2TP_CMD_SESSION_CREATE: %w", err)
	}
	log.Printf("Kernel L2TP session registered: sid=%d psid=%d", sessionID, peerSessionID)
	return nil
}

// UnregisterSession deletes the session from the kernel.
func (k *kernelL2TP) UnregisterSession(tunnelID, sessionID uint32) error {
	attrs := []netlink.Attribute{
		{Type: l2tpAttrConnId, Data: nlenc.Uint32Bytes(tunnelID)},
		{Type: l2tpAttrSessionId, Data: nlenc.Uint32Bytes(sessionID)},
	}
	data, err := netlink.MarshalAttributes(attrs)
	if err != nil {
		return err
	}
	req := genetlink.Message{
		Header: genetlink.Header{Command: l2tpCmdSessionDelete, Version: l2tpGenlVersion},
		Data:   data,
	}
	_, err = k.conn.Execute(req, k.familyID, netlink.Request|netlink.Acknowledge)
	if err != nil {
		return fmt.Errorf("L2TP_CMD_SESSION_DELETE: %w", err)
	}
	return nil
}

// UnregisterTunnel deletes the tunnel from the kernel (sessions are destroyed with it).
func (k *kernelL2TP) UnregisterTunnel(tunnelID uint32) error {
	attrs := []netlink.Attribute{
		{Type: l2tpAttrConnId, Data: nlenc.Uint32Bytes(tunnelID)},
	}
	data, err := netlink.MarshalAttributes(attrs)
	if err != nil {
		return err
	}
	req := genetlink.Message{
		Header: genetlink.Header{Command: l2tpCmdTunnelDelete, Version: l2tpGenlVersion},
		Data:   data,
	}
	_, err = k.conn.Execute(req, k.familyID, netlink.Request|netlink.Acknowledge)
	if err != nil {
		return fmt.Errorf("L2TP_CMD_TUNNEL_DELETE: %w", err)
	}
	return nil
}

// Close closes the netlink connection.
func (k *kernelL2TP) Close() error {
	if k.conn != nil {
		return k.conn.Close()
	}
	return nil
}
