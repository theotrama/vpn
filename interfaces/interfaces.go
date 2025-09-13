package interfaces

import "net"

type IPv4 struct {
	Version        uint8
	IHL            uint8
	DSCP           uint8
	ECN            uint8
	TotalLength    uint16
	Identification uint16
	Flags          uint8
	FragmentOffset uint16
	TimeToLive     uint8
	Protocol       uint8
	HeaderChecksum uint16
	SrcAddr        net.IP
	DestAddr       net.IP
}

type UDP struct {
	SrcPort  uint16
	DestPort uint16
	Length   uint16
	Checksum uint16
}

const (
	IPv4HeaderLength = 20
)
