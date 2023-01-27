package core

import (
	"time"

	"github.com/karlseguin/ccache/v3"
)

const udpIdleTimeout = time.Second * 300

// mac MaxSize = 4096 will crash
var udpConns = ccache.New(ccache.Configure[UDPConn]().MaxSize(64).OnDelete(func(item *ccache.Item[UDPConn]) {
	item.Value().Close()
}))

func init() {
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
