package core

import "C"
import (
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
