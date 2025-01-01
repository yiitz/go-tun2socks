package core

/*
#cgo CFLAGS: -I./c/include
#include "lwip/ip_addr.h"
#include <string.h>
*/
import "C"
import (
	"net"
	"unsafe"
)

func UnsafeStringToBytes(s string) []byte {
	if s == "" {
		return nil // or []byte{}
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

func UnsafeBytesToString(bs []byte) string {
	if len(bs) == 0 {
		return ""
	}
	return unsafe.String(&bs[0], len(bs))
}

func UnsafeStringToCharPtr(s string) *C.char {
	return (*C.char)(unsafe.Pointer(unsafe.StringData(s)))
}

func UnsafeGoIPToC(ip net.IP, cAddr *C.struct_ip_addr) {
	if ipv4 := ip.To4(); ipv4 != nil {
		cAddr._type = C.uint8_t(0)
		C.memcpy(unsafe.Pointer(&cAddr.u_addr), unsafe.Pointer(&ipv4[0]), C.size_t(4))
	} else if ipv6 := ip.To16(); ipv6 != nil {
		cAddr._type = C.uint8_t(6)
		C.memcpy(unsafe.Pointer(&cAddr.u_addr), unsafe.Pointer(&ipv6[0]), C.size_t(16))
	} else {
		panic("invalid ip address")
	}
}
