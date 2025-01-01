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
	"fmt"
	"net"
	"strings"
	"time"
	"unsafe"

	lru "github.com/hashicorp/golang-lru/v2"
)

var ipCacheTimeout = time.Minute

var udpConns *lru.Cache[string, UDPConn]

type ipCacheItem struct {
	value unsafe.Pointer
	t     time.Time
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
func GetUDPConnStats() string {
	var stats strings.Builder
	fmt.Fprintf(&stats, "udp connection count: %d, list:\n", udpConns.Len())
	for k, conn := range udpConns.Values() {
		fmt.Fprintln(&stats, fmt.Sprintf("conn %d: ", k), conn.LocalAddr().String())
	}
	return stats.String()
}

func init() {
	maxConnSize := 1024
	udpConns, _ = lru.NewWithEvict(maxConnSize, func(key string, value UDPConn) {
		value.Close()
	})
	ipCache, _ = lru.NewWithEvict(maxConnSize, func(key string, value *ipCacheItem) {
		C.free_struct_ip_addr(value.value)
	})

	t := time.NewTicker(time.Second * 10)
	go func() {
		for now := range t.C {
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
