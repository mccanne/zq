package zed

import (
	"errors"
	"net"

	"github.com/brimdata/zed/zcode"
)

type TypeOfNet struct{}

func NewNet(s *net.IPNet) Value {
	return Value{TypeNet, EncodeNet(s)}
}

func AppendNet(zb zcode.Bytes, subnet *net.IPNet) zcode.Bytes {
	if ip := subnet.IP.To4(); ip != nil {
		zb = append(zb, ip...)
		if len(subnet.Mask) == 16 {
			return append(zb, subnet.Mask[12:]...)
		}
		return append(zb, subnet.Mask...)
	}
	zb = append(zb, subnet.IP...)
	return append(zb, subnet.Mask...)
}

func EncodeNet(subnet *net.IPNet) zcode.Bytes {
	return AppendNet(nil, subnet)
}

func DecodeNet(zv zcode.Bytes) (*net.IPNet, error) {
	if zv == nil {
		return nil, nil
	}
	switch len(zv) {
	case 8:
		ip := net.IP(zv[:4])
		mask := net.IPMask(zv[4:])
		return &net.IPNet{
			IP:   ip,
			Mask: mask,
		}, nil
	case 32:
		ip := net.IP(zv[:16])
		mask := net.IPMask(zv[16:])
		return &net.IPNet{
			IP:   ip,
			Mask: mask,
		}, nil
	}
	return nil, errors.New("failure trying to decode IP subnet that is not 8 or 32 bytes long")
}

func (t *TypeOfNet) ID() int {
	return IDNet
}

func (t *TypeOfNet) String() string {
	return "net"
}

func (t *TypeOfNet) Marshal(zv zcode.Bytes) (interface{}, error) {
	s, err := DecodeNet(zv)
	if err != nil {
		return nil, err
	}
	return (*s).String(), nil
}

func (t *TypeOfNet) Format(zv zcode.Bytes) string {
	s, err := DecodeNet(zv)
	if err != nil {
		return badZng(err, t, zv)
	}
	ipnet := net.IPNet(*s)
	return ipnet.String()
}
