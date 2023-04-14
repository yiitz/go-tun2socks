package core

import (
	"reflect"
	"unsafe"
)

func UnsafeStringToBytes(s string) []byte {
	if s == "" {
		return nil // or []byte{}
	}
	return unsafe.Slice((*byte)(unsafe.Pointer((*reflect.StringHeader)(unsafe.Pointer(&s)).Data)), len(s))
}

func UnsafeBytesToString(bs []byte) string {
	if len(bs) == 0 {
		return ""
	}
	return *(*string)(unsafe.Pointer(&bs))
}
