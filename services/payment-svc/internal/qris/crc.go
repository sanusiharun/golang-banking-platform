package qris

import "fmt"

// crc16CCITT computes the CRC-16/CCITT-FALSE checksum of data:
//
//	polynomial 0x1021, initial value 0xFFFF, no input/output reflection,
//	no final XOR.
//
// This is the checksum algorithm mandated by the EMVCo MPM specification for
// tag 63. The canonical check value for the input "123456789" is 0x29B1.
func crc16CCITT(data string) uint16 {
	crc := uint16(0xFFFF)
	for i := 0; i < len(data); i++ {
		crc ^= uint16(data[i]) << 8
		for bit := 0; bit < 8; bit++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// CRC16 returns the EMVCo tag-63 checksum of data as a 4-character uppercase
// hex string (e.g. "A13A").
func CRC16(data string) string {
	return fmt.Sprintf("%04X", crc16CCITT(data))
}
