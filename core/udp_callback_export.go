package core

/*
#cgo CFLAGS: -I./c/include
#include "lwip/udp.h"
*/
import "C"
import (
	"io"
	"net"
	"strconv"
	"unsafe"
)

//export udpRecvFn
func udpRecvFn(arg unsafe.Pointer, pcb *C.struct_udp_pcb, p *C.struct_pbuf, addr *C.ip_addr_t, port C.u16_t, destAddr *C.ip_addr_t, destPort C.u16_t) {
	defer func() {
		if p != nil {
			C.pbuf_free(p)
		}
	}()

	if pcb == nil {
		return
	}

	srcIP := ipAddrNTOA(*addr) // TODO 减少复制
	destIP := ipAddrNTOA(*destAddr)
	if srcIP == "" || destIP == "" {
		panic("invalid UDP address")
	}
	dstAddr := ParseUDPAddr(net.JoinHostPort(destIP, strconv.Itoa(int(destPort))))

	connId := net.JoinHostPort(srcIP, strconv.Itoa(int(port))) // + "-" + ipAddrNTOA(*destAddr) + ":" + strconv.Itoa(int(uint16(destPort)))
	conn, ok := udpConns.Get(connId)
	if !ok {
		if udpConnHandler == nil {
			panic("must register a UDP connection handler")
		}
		srcAddr := ParseUDPAddr(connId)
		var err error
		if h2, ok := udpConnHandler.(UDPConnHandlerEx); ok {
			conn, err = newUDPConnEx(connId, pcb,
				h2,
				*addr,
				port,
				srcAddr,
				dstAddr)
			if err != nil {
				return
			}
		} else {
			conn, err = newUDPConn(connId, pcb,
				udpConnHandler,
				*addr,
				port,
				srcAddr,
				dstAddr)
			if err != nil {
				return
			}
		}
	}
	var totlen = int(p.tot_len)
	if connex, ok := conn.(*udpConnex); ok {
		var pbr = &pbbufReader{p: p}
		connex.ReceiveToBuffer(pbr, dstAddr)
	} else {
		var buf []byte
		if p.tot_len == p.len {
			buf = (*[1 << 30]byte)(unsafe.Pointer(p.payload))[:totlen:totlen]
		} else {
			buf = NewBytes(totlen)
			defer FreeBytes(buf)
			C.pbuf_copy_partial(p, unsafe.Pointer(&buf[0]), p.tot_len, 0)
		}
		conn.ReceiveTo(buf[:totlen], dstAddr)
	}

}

type pbbufReader struct {
	p      *C.struct_pbuf
	offset int
}

func (me *pbbufReader) Read(buf []byte) (n int, err error) {
	p := me.p
	var totlen = int(p.tot_len)
	left := totlen - me.offset
	if left <= 0 {
		return 0, io.EOF
	}
	if p.tot_len == p.len {
		if left > len(buf) {
			n = len(buf)
		} else {
			n = left
		}
		copy(buf, (*[1 << 30]byte)(unsafe.Pointer(p.payload))[me.offset:me.offset+n])
		me.offset += n
	} else {
		if left > len(buf) {
			n = len(buf)
		} else {
			n = left
		}
		C.pbuf_copy_partial(p, unsafe.Pointer(&buf[0]), C.u16_t(n), C.u16_t(me.offset))
		me.offset += n
	}
	return
}

func (me *pbbufReader) Len() int {
	return int(me.p.tot_len) - me.offset
}

func (me *pbbufReader) Cap() int {
	return int(me.p.tot_len)
}
