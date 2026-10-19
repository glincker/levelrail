// Package walship is an opt-in prototype of continuous SQLite replication
// by WAL frame shipping (ADR 039). It is not wired into the control plane.
package walship

import (
	"encoding/binary"
	"errors"
)

const (
	walHeaderSize  = 32
	frameHeaderLen = 24
	magicLE        = 0x377f0682
	magicBE        = 0x377f0683
)

var errBadWAL = errors.New("walship: invalid WAL header")

type walHeader struct {
	raw      [walHeaderSize]byte
	bigEnd   bool
	pageSize int
	seq      uint32
	salt1    uint32
	salt2    uint32
	cksum    [2]uint32
}

func parseHeader(b []byte) (walHeader, error) {
	var h walHeader
	if len(b) < walHeaderSize {
		return h, errBadWAL
	}
	copy(h.raw[:], b[:walHeaderSize])
	switch binary.BigEndian.Uint32(b[0:4]) {
	case magicLE:
	case magicBE:
		h.bigEnd = true
	default:
		return h, errBadWAL
	}
	h.pageSize = int(binary.BigEndian.Uint32(b[8:12]))
	h.seq = binary.BigEndian.Uint32(b[12:16])
	h.salt1 = binary.BigEndian.Uint32(b[16:20])
	h.salt2 = binary.BigEndian.Uint32(b[20:24])
	h.cksum[0] = binary.BigEndian.Uint32(b[24:28])
	h.cksum[1] = binary.BigEndian.Uint32(b[28:32])
	if walChecksum(h.bigEnd, 0, 0, b[:24]) != h.cksum || h.pageSize < 512 {
		return h, errBadWAL
	}
	return h, nil
}

func walChecksum(bigEnd bool, s0, s1 uint32, b []byte) [2]uint32 {
	for i := 0; i+8 <= len(b); i += 8 {
		var x0, x1 uint32
		if bigEnd {
			x0 = binary.BigEndian.Uint32(b[i:])
			x1 = binary.BigEndian.Uint32(b[i+4:])
		} else {
			x0 = binary.LittleEndian.Uint32(b[i:])
			x1 = binary.LittleEndian.Uint32(b[i+4:])
		}
		s0 += x0 + s1
		s1 += x1 + s0
	}
	return [2]uint32{s0, s1}
}

// scanFrames validates frames in buf (WAL bytes after the header, starting
// at a frame boundary) and returns the byte length ending at the last
// commit frame plus the cumulative checksum at that point.
func scanFrames(h walHeader, prev [2]uint32, buf []byte) (int, [2]uint32) {
	frameSize := frameHeaderLen + h.pageSize
	good := 0
	goodSum := prev
	sum := prev
	for off := 0; off+frameSize <= len(buf); off += frameSize {
		fh := buf[off : off+frameHeaderLen]
		if binary.BigEndian.Uint32(fh[8:12]) != h.salt1 || binary.BigEndian.Uint32(fh[12:16]) != h.salt2 {
			break
		}
		sum = walChecksum(h.bigEnd, sum[0], sum[1], fh[:8])
		sum = walChecksum(h.bigEnd, sum[0], sum[1], buf[off+frameHeaderLen:off+frameSize])
		if binary.BigEndian.Uint32(fh[16:20]) != sum[0] || binary.BigEndian.Uint32(fh[20:24]) != sum[1] {
			break
		}
		if binary.BigEndian.Uint32(fh[4:8]) != 0 {
			good = off + frameSize
			goodSum = sum
		}
	}
	return good, goodSum
}
