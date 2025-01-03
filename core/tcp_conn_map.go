package core

/*
#cgo CFLAGS: -I./c/include
#include "lwip/tcp.h"
#include <stdlib.h>

void*
new_conn_key_arg()
{
	return malloc(sizeof(uint32_t));
}

void
free_conn_key_arg(void *arg)
{
	free(arg);
}

void
set_conn_key_val(void *arg, uint32_t val)
{
	*((uint32_t*)arg) = val;
}

uint32_t
get_conn_key_val(void *arg)
{
	return *((uint32_t*)arg);
}
*/
import "C"
import (
	"fmt"
	"io"
	"unsafe"

	lru "github.com/hashicorp/golang-lru/v2"
)

var tcpConns *lru.Cache[uint32, TCPConn]

var connKeyArgCounter uint32 = 1

func SetTCPParams(maxConnSize int) {
	if maxConnSize > 0 {
		tcpConns.Resize(maxConnSize)
	}
}
func WriteTCPConnStats(w io.Writer) {
	fmt.Fprintf(w, "tcp connection count: %d, list:\n", tcpConns.Len())
	for k, conn := range tcpConns.Values() {
		fmt.Fprintln(w, fmt.Sprintf("conn %d: ", k), conn.LocalAddr().String(), " -> ", conn.RemoteAddr().String())
	}
}

// We need such a key-value mechanism because when passing a Go pointer
// to C, the Go pointer will only be valid during the call.
// If we pass a Go pointer to tcp_arg(), this pointer will not be usable
// in subsequent callbacks (e.g.: tcp_recv(), tcp_err()).
//
// Instead we need to pass a C pointer to tcp_arg(), we manually allocate
// the memory in C and return its pointer to Go code. After the connection
// end, the memory should be freed manually.
//
// See also:
// https://github.com/golang/go/issues/12416
func newConnKeyArg() unsafe.Pointer {
	return C.new_conn_key_arg()
}

func freeConnKeyArg(p unsafe.Pointer) {
	C.free_conn_key_arg(p)
}

func setConnKeyVal(p unsafe.Pointer, val uint32) {
	C.set_conn_key_val(p, C.uint32_t(val))
}

func getConnKeyVal(p unsafe.Pointer) uint32 {
	return uint32(C.get_conn_key_val(p))
}

func getNextConnKeyVal() uint32 {
	connKey := connKeyArgCounter
	connKeyArgCounter += 1
	return connKey
}

func init() {
	maxConnSize := 1024
	tcpConns, _ = lru.NewWithEvict(maxConnSize, func(key uint32, value TCPConn) {
		go value.Abort()
	})
}
