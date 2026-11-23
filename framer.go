package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

// 幀類型（①：控制/數據幀隔離）。
const (
	frameTypeHandshake = 0x01 // 握手請求/響應（僅在連接建立階段）
	frameTypeKeepalive = 0x02 // 保活（無載荷）
	frameTypeEthernet  = 0x03 // 二層以太網幀（數據面）
)

// bufPool 复用帧写缓冲，降低 GC 压力。
var bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 4096)
		return &b
	},
}

// validFrameType 判斷幀類型是否合法。
func validFrameType(t byte) bool {
	return t == frameTypeHandshake || t == frameTypeKeepalive || t == frameTypeEthernet
}

// writeFrame 把 data 以 [type:1B][len:uint16][payload] 的幀格式寫入連接。
func writeFrame(conn net.Conn, frameType byte, data []byte) error {
	payloadLen := len(data)
	if payloadLen > 0xFFFF {
		return fmt.Errorf("數據幀過大: %d", payloadLen)
	}
	if !validFrameType(frameType) {
		return fmt.Errorf("無效幀類型: %#x", frameType)
	}

	total := 3 + payloadLen
	bufPtr := bufPool.Get().(*[]byte)
	defer bufPool.Put(bufPtr)

	buf := *bufPtr
	if cap(buf) < total {
		// 超出池容量（罕见），重新分配且不入池。
		buf = make([]byte, total)
	} else {
		buf = buf[:total]
	}

	buf[0] = frameType
	binary.BigEndian.PutUint16(buf[1:3], uint16(payloadLen))
	if payloadLen > 0 {
		copy(buf[3:], data)
	}

	_, err := conn.Write(buf)
	return err
}

// readFrame 從連接讀取一幀，返回 (幀類型, 載荷)。
// maxLen 為允許的最大載荷長度，超過則報錯（防禦異常/惡意幀）。
func readFrame(conn net.Conn, maxLen int) (byte, []byte, error) {
	var headerBuf [3]byte
	if _, err := io.ReadFull(conn, headerBuf[:]); err != nil {
		return 0, nil, err
	}

	frameType := headerBuf[0]
	if !validFrameType(frameType) {
		return 0, nil, fmt.Errorf("無效幀類型: %#x", frameType)
	}

	payloadLen := int(binary.BigEndian.Uint16(headerBuf[1:3]))
	if payloadLen > maxLen {
		return 0, nil, fmt.Errorf("數據幀過大: %d (限制: %d)", payloadLen, maxLen)
	}
	if payloadLen == 0 {
		return frameType, []byte{}, nil
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return 0, nil, err
	}
	return frameType, payload, nil
}
