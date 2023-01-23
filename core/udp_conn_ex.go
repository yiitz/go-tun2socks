package core

/*
#cgo CFLAGS: -I./c/include
#include "lwip/udp.h"
*/
import "C"
import (
	"errors"
	"io"
	"net"
	"sync/atomic"
	"unsafe"
)

type udpConnex struct {
	connId    string
	pcb       *C.struct_udp_pcb
	handler   UDPConnHandlerEx
	localAddr *net.UDPAddr
	localIP   C.ip_addr_t
	localPort C.u16_t
	closed    atomic.Bool
	data      interface{}
}

func newUDPConnEx(connId string, pcb *C.struct_udp_pcb, handler UDPConnHandlerEx, localIP C.ip_addr_t, localPort C.u16_t, localAddr, remoteAddr *net.UDPAddr) (UDPConn, error) {
	conn := &udpConnex{
		connId:    connId,
		handler:   handler,
		pcb:       pcb,
		localAddr: localAddr,
		localIP:   localIP,
		localPort: localPort,
	}

	err := handler.Connect(conn, remoteAddr)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (conn *udpConnex) LocalAddr() *net.UDPAddr {
	return conn.localAddr
}

func (conn *udpConnex) checkState() error {
	if conn.closed.Load() {
		return errors.New("connection closed")
	}
	return nil
}

func (conn *udpConnex) ReceiveTo(data []byte, addr *net.UDPAddr) error {
	return errors.New("user ReceiveToBuffer")
}

func (conn *udpConnex) ReceiveToBuffer(reader BytesReader, addr *net.UDPAddr) error {
	return conn.handler.ReceiveToBuffer(conn, reader, addr)
}

func (conn *udpConnex) WriteFrom(data []byte, addr *net.UDPAddr) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if err := conn.checkState(); err != nil {
		return 0, err
	}
	// FIXME any memory leaks?
	cremoteIP := C.struct_ip_addr{}
	if err := ipAddrATON(addr.IP.String(), &cremoteIP); err != nil {
		return 0, err
	}
	buf := C.pbuf_alloc_reference(unsafe.Pointer(&data[0]), C.u16_t(len(data)), C.PBUF_ROM)
	defer C.pbuf_free(buf)
	C.udp_sendto(conn.pcb, buf, &conn.localIP, conn.localPort, &cremoteIP, C.u16_t(addr.Port))
	item := udpConns.Get(conn.connId)
	if item != nil {
		item.Extend(udpIdleTimeout)
	}
	return len(data), nil
}

func (conn *udpConnex) Close() error {
	if conn.closed.CompareAndSwap(false, true) {
		udpConns.Delete(conn.connId)
		if o, ok := conn.data.(io.Closer); ok {
			o.Close()
		}
	}
	return nil
}

func (conn *udpConnex) SetData(data interface{}) {
	conn.data = data
}

func (conn *udpConnex) GetData() interface{} {
	return conn.data
}
