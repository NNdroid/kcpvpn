package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

var bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 4096)
		return &b
	},
}

func applyXOR(data []byte, key []byte) {
	if len(key) == 0 {
		return
	}
	for i := range data {
		data[i] ^= key[i%len(key)]
	}
}

func getObfsHeaderLen(mode string) int {
	switch mode {
	case "stun":
		return 20
	case "dns":
		return 12
	case "dtls":
		return 13
	case "quic":
		return 8
	default:
		return 0
	}
}

func wrapObfs(payload []byte, mode string, key []byte) []byte {
	switch mode {
	case "stun":
		hdr := make([]byte, 20+len(payload))
		binary.BigEndian.PutUint16(hdr[0:2], 0x0101) // STUN Binding Success Response
		binary.BigEndian.PutUint16(hdr[2:4], uint16(len(payload)))
		binary.BigEndian.PutUint32(hdr[4:8], 0x2112A442) // STUN Magic Cookie
		if len(key) >= 12 {
			copy(hdr[8:20], key[:12])
		} else {
			rand.Read(hdr[8:20])
		}
		copy(hdr[20:], payload)
		return hdr

	case "dns":
		hdr := make([]byte, 12+len(payload))
		binary.BigEndian.PutUint16(hdr[0:2], 0x1234) // Transaction ID
		binary.BigEndian.PutUint16(hdr[2:4], 0x8180) // Flags: Standard query response
		binary.BigEndian.PutUint16(hdr[4:6], 1)      // Questions
		binary.BigEndian.PutUint16(hdr[6:8], 1)      // Answers
		binary.BigEndian.PutUint16(hdr[8:10], 0)
		binary.BigEndian.PutUint16(hdr[10:12], 0)
		copy(hdr[12:], payload)
		return hdr

	case "dtls":
		hdr := make([]byte, 13+len(payload))
		hdr[0] = 0x17                                // ContentType: Application Data
		binary.BigEndian.PutUint16(hdr[1:3], 0xFEFF) // Version: DTLS 1.2
		binary.BigEndian.PutUint16(hdr[3:5], 0x0001) // Epoch
		hdr[5] = 0
		hdr[6] = 0
		hdr[7] = 0
		hdr[8] = 0
		hdr[9] = 0
		hdr[10] = 1
		binary.BigEndian.PutUint16(hdr[11:13], uint16(len(payload)))
		copy(hdr[13:], payload)
		return hdr

	case "quic":
		hdr := make([]byte, 8+len(payload))
		hdr[0] = 0x40 // QUIC Short Header
		if len(key) >= 4 {
			copy(hdr[1:5], key[:4])
		}
		hdr[5] = 0x00
		hdr[6] = 0x00
		hdr[7] = 0x01
		copy(hdr[8:], payload)
		return hdr

	case "xor":
		applyXOR(payload, key)
		return payload

	default:
		return payload
	}
}

func writeFrame(conn net.Conn, data []byte, obfsMode string, key []byte, padMax int) error {
	payloadLen := len(data)
	padLen := 0
	if padMax > 0 {
		var p [1]byte
		rand.Read(p[:])
		padLen = int(p[0]) % (padMax + 1)
	}

	totalInner := 2 + 1 + payloadLen + padLen
	innerBuf := make([]byte, totalInner)

	binary.BigEndian.PutUint16(innerBuf[0:2], uint16(payloadLen))
	innerBuf[2] = byte(padLen)
	if payloadLen > 0 {
		copy(innerBuf[3:], data)
	}
	if padLen > 0 {
		rand.Read(innerBuf[3+payloadLen:])
	}

	finalBuf := wrapObfs(innerBuf, obfsMode, key)
	_, err := conn.Write(finalBuf)
	return err
}

func readFrame(conn net.Conn, maxFrameLen int, obfsMode string, key []byte) ([]byte, error) {
	headerLen := getObfsHeaderLen(obfsMode)
	if headerLen > 0 {
		obfsHdr := make([]byte, headerLen)
		if _, err := io.ReadFull(conn, obfsHdr); err != nil {
			return nil, err
		}
	}

	var headerBuf [3]byte // 2 bytes payload len + 1 byte pad len
	if _, err := io.ReadFull(conn, headerBuf[:]); err != nil {
		return nil, err
	}

	if obfsMode == "xor" {
		applyXOR(headerBuf[:], key)
	}

	payloadLen := int(binary.BigEndian.Uint16(headerBuf[0:2]))
	padLen := int(headerBuf[2])

	if payloadLen > maxFrameLen*2 {
		return nil, fmt.Errorf("數據幀過大: %d (限制: %d)", payloadLen, maxFrameLen*2)
	}

	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return nil, err
		}
		if obfsMode == "xor" {
			applyXOR(payload, key[3:])
		}
	} else {
		payload = []byte{}
	}

	if padLen > 0 {
		discard := make([]byte, padLen)
		if _, err := io.ReadFull(conn, discard); err != nil {
			return nil, err
		}
	}

	return payload, nil
}
