package core

import "C"
import (
	"reflect"
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

func UnsafeStringToCharPtr(goStr string) *C.char {
	// 获取字符串底层的指针
	strHeader := (*reflect.StringHeader)(unsafe.Pointer(&goStr))
	return (*C.char)(unsafe.Pointer(strHeader.Data))
}
