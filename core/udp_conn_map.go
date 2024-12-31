package core

/*
#cgo CFLAGS: -I./c/include
#include "lwip/udp.h"
#include <stdlib.h>

void*
new_struct_ip_addr()
{
	return malloc(sizeof(ip_addr_t));
}

void
free_struct_ip_addr(void *arg)
{
	free(arg);
}

*/
import "C"
import (
	"runtime"
	"time"
	"unsafe"

	"github.com/karlseguin/ccache/v3"
)

var ipCacheTimeout = time.Minute * 10

var udpConns *ccache.Cache[UDPConn]

var ipCache *ccache.Cache[unsafe.Pointer]

func init() {
	maxConnSize := int64(1024)
	switch runtime.GOOS {
	case "darwin", "ios":
		maxConnSize = 192
	}
	udpConns = ccache.New(ccache.Configure[UDPConn]().MaxSize(maxConnSize).OnDelete(func(item *ccache.Item[UDPConn]) {
		item.Value().CloseOnly()
	}))
	ipCache = ccache.New(ccache.Configure[unsafe.Pointer]().MaxSize(maxConnSize).OnDelete(func(item *ccache.Item[unsafe.Pointer]) {
		C.free_struct_ip_addr(item.Value())
	}))

	t := time.NewTicker(ipCacheTimeout / 2)
	go func() {
		for range t.C {
			now := time.Now()
			ipCache.DeleteFunc(func(_ string, item *ccache.Item[unsafe.Pointer]) bool {
				return now.After(item.Expires())
			})
		}
	}()
}

func SetUDPParams(maxConnSize int64, udpIdleTimeout, dnsUdpIdleTimeout time.Duration) {
	if maxConnSize > 0 {
		udpConns.SetMaxSize(maxConnSize)
		ipCache.SetMaxSize(maxConnSize)
	}
}
