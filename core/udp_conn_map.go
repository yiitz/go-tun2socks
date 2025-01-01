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
	"errors"
	"net"
	"runtime"
	"time"
	"unsafe"

	lru "github.com/hashicorp/golang-lru/v2"
)

var ipCacheTimeout = time.Minute

var udpConns *lru.Cache[string, UDPConn]

type ipCacheItem struct {
	t     time.Time
	value unsafe.Pointer
}

var ipCache *lru.Cache[string, *ipCacheItem]

func SetUDPParams(maxConnSize int, ipCacheTimeoutParam time.Duration) {
	if maxConnSize > 0 {
		udpConns.Resize(maxConnSize)
		ipCache.Resize(maxConnSize)
	}
	if ipCacheTimeout > 0 {
		ipCacheTimeout = ipCacheTimeoutParam
	}
}

func init() {
	maxConnSize := 1024
	switch runtime.GOOS {
	case "darwin", "ios":
		maxConnSize = 192
	}
	udpConns, _ = lru.NewWithEvict(maxConnSize, func(key string, value UDPConn) {
		value.CloseOnly()
	})
	ipCache, _ = lru.NewWithEvict(maxConnSize, func(key string, value *ipCacheItem) {
		C.free_struct_ip_addr(value.value)
	})

	t := time.NewTicker(time.Second * 10)
	go func() {
		for range t.C {
			now := time.Now()
			for {
				k, v, ok := ipCache.GetOldest()
				if !ok {
					break
				}
				if v.t.Add(ipCacheTimeout).After(now) {
					break
				}
				ipCache.Remove(k)
			}
		}
	}()
}

func newIpCacheItem(ip net.IP) (*ipCacheItem, error) {
	v := C.new_struct_ip_addr()
	if v == nil {
		return nil, errors.New("malloc struct_ip_addr failed")
	}
	if err := ipAddrATON(ip.String(), (*C.struct_ip_addr)(v)); err != nil {
		return nil, err
	}
	return &ipCacheItem{value: v}, nil
}
