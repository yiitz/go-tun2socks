package core

import (
	"fmt"
	"io"

	lru "github.com/hashicorp/golang-lru/v2"
)

var udpConns *lru.Cache[string, UDPConn]

func SetUDPParams(maxConnSize int) {
	if maxConnSize > 0 {
		udpConns.Resize(maxConnSize)
	}
}
func WriteUDPConnStats(w io.Writer) {
	fmt.Fprintf(w, "udp connection count: %d, list:\n", udpConns.Len())
	for k, conn := range udpConns.Values() {
		fmt.Fprintln(w, fmt.Sprintf("conn %d: ", k), conn.LocalAddr().String())
	}
}

func init() {
	maxConnSize := 1024
	udpConns, _ = lru.NewWithEvict(maxConnSize, func(key string, value UDPConn) {
		value.Close()
	})
}
