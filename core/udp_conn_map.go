package core

import (
	"runtime"
	"time"

	"github.com/karlseguin/ccache/v3"
)

var _udpIdleTimeout = time.Second * 60
var _dnsUdpIdleTimeout = time.Second * 10

// mac MaxSize = 4096 will crash
var udpConns *ccache.Cache[UDPConn]

func init() {
	maxConnSize := int64(1024)
	switch runtime.GOOS {
	case "darwin", "ios":
		maxConnSize = 192
	}
	udpConns = ccache.New(ccache.Configure[UDPConn]().MaxSize(maxConnSize).OnDelete(func(item *ccache.Item[UDPConn]) {
		item.Value().Close()
	}))
	go func() {
		for {
			time.Sleep(time.Second * 30)
			now := time.Now()
			udpConns.DeleteFunc(func(_ string, item *ccache.Item[UDPConn]) bool {
				return now.After(item.Expires())
			})
		}
	}()
}

func SetUDPParams(maxConnSize int64, udpIdleTimeout, dnsUdpIdleTimeout time.Duration) {
	if udpIdleTimeout > 0 {
		_udpIdleTimeout = udpIdleTimeout
	}
	if dnsUdpIdleTimeout > 0 {
		_dnsUdpIdleTimeout = dnsUdpIdleTimeout
	}
	if maxConnSize > 0 {
		udpConns.SetMaxSize(maxConnSize)
	}
}
