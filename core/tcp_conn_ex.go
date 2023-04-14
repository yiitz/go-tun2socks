package core

/*
#cgo CFLAGS: -I./c/include
#include "lwip/tcp.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"
	"time"
	"unsafe"
)

type tcpConnEx struct {
	sync.Mutex

	pcb        *C.struct_tcp_pcb
	handler    TCPConnHandler
	remoteAddr *net.TCPAddr
	localAddr  *net.TCPAddr
	connKeyArg unsafe.Pointer
	connKey    uint32
	canWrite   *sync.Cond // Condition variable to implement TCP backpressure.
	state      tcpConnState
	closeOnce  sync.Once
	closeErr   error
	patch      TCPConnPatch
}

func newTCPConnEx(pcb *C.struct_tcp_pcb, handler TCPConnHandlerEx) (TCPConnEx, error) {
	connKeyArg := newConnKeyArg()
	connKey := rand.Uint32()
	setConnKeyVal(unsafe.Pointer(connKeyArg), connKey)

	// Pass the key as arg for subsequent tcp callbacks.
	C.tcp_arg(pcb, unsafe.Pointer(connKeyArg))

	// Register callbacks.
	setTCPRecvCallback(pcb)
	setTCPSentCallback(pcb)
	setTCPErrCallback(pcb)
	setTCPPollCallback(pcb, C.u8_t(TCP_POLL_INTERVAL))

	conn := &tcpConnEx{
		pcb:        pcb,
		handler:    handler,
		localAddr:  ParseTCPAddr(ipAddrNTOA(pcb.remote_ip), uint16(pcb.remote_port)),
		remoteAddr: ParseTCPAddr(ipAddrNTOA(pcb.local_ip), uint16(pcb.local_port)),
		connKeyArg: connKeyArg,
		connKey:    connKey,
		canWrite:   sync.NewCond(&sync.Mutex{}),
		state:      tcpNewConn,
	}

	// Associate conn with key and save to the global map.
	tcpConns.Store(connKey, conn)
	conn.state = tcpConnecting
	conn.patch = handler.HandleEx(conn, conn.remoteAddr)
	conn.state = tcpConnected
	if pcb.refused_data != nil {
		C.tcp_process_refused_data(pcb)
	}

	return conn, NewLWIPError(LWIP_ERR_OK)
}

func (conn *tcpConnEx) SetPatch(p TCPConnPatch) {
	conn.patch = p
}
func (conn *tcpConnEx) GetPatch() TCPConnPatch {
	return conn.patch
}

func (conn *tcpConnEx) RemoteAddr() net.Addr {
	return conn.remoteAddr
}

func (conn *tcpConnEx) LocalAddr() net.Addr {
	return conn.localAddr
}

func (conn *tcpConnEx) SetDeadline(t time.Time) error {
	return nil
}
func (conn *tcpConnEx) SetReadDeadline(t time.Time) error {
	return nil
}
func (conn *tcpConnEx) SetWriteDeadline(t time.Time) error {
	return nil
}

func (conn *tcpConnEx) receiveCheck() error {
	conn.Lock()
	defer conn.Unlock()

	switch conn.state {
	case tcpConnected:
		fallthrough
	case tcpWriteClosed:
		return nil
	case tcpNewConn:
		fallthrough
	case tcpConnecting:
		return NewLWIPError(LWIP_ERR_CONN)
	case tcpAborting:
		fallthrough
	case tcpClosed:
		fallthrough
	case tcpReceiveClosed:
		fallthrough
	case tcpClosing:
		return NewLWIPError(LWIP_ERR_CLSD)
	case tcpErrored:
		conn.abortInternal()
		return NewLWIPError(LWIP_ERR_ABRT)
	default:
		panic("unexpected error")
	}
}

func (conn *tcpConnEx) Receive(data []byte) error {
	panic("handle by ReceiveBuffer")
}

func (conn *tcpConnEx) ReceiveBuffer(reader BytesReader) error {
	if err := conn.receiveCheck(); err != nil {
		return err
	}
	n, err := conn.patch.ReceiveEx(reader)
	if err != nil {
		return NewLWIPError(LWIP_ERR_CLSD)
	}
	C.tcp_recved(conn.pcb, C.u16_t(n))
	return NewLWIPError(LWIP_ERR_OK)
}

func (conn *tcpConnEx) Read(data []byte) (int, error) {
	panic("handled by ReceiveBuffer")
}

// writeInternal enqueues data to snd_buf, and treats ERR_MEM returned by tcp_write not an error,
// but instead tells the caller that data is not successfully enqueued, and should try
// again another time. By calling this function, the lwIP thread is assumed to be already
// locked by the caller.
func (conn *tcpConnEx) writeInternal(data []byte) (int, error) {
	err := C.tcp_write(conn.pcb, unsafe.Pointer(&data[0]), C.u16_t(len(data)), C.TCP_WRITE_FLAG_COPY)
	if err == C.ERR_OK {
		C.tcp_output(conn.pcb)
		return len(data), nil
	} else if err == C.ERR_MEM {
		return 0, nil
	}
	return 0, fmt.Errorf("tcp_write failed (%v)", int(err))
}

func (conn *tcpConnEx) writeCheck() error {
	conn.Lock()
	defer conn.Unlock()

	switch conn.state {
	case tcpConnecting:
		fallthrough
	case tcpConnected:
		fallthrough
	case tcpReceiveClosed:
		return nil
	case tcpWriteClosed:
		fallthrough
	case tcpClosing:
		fallthrough
	case tcpClosed:
		fallthrough
	case tcpErrored:
		fallthrough
	case tcpAborting:
		return io.ErrClosedPipe
	default:
		panic("unexpected error")
	}
}

func (conn *tcpConnEx) Write(data []byte) (int, error) {
	totalWritten := 0

	conn.canWrite.L.Lock()
	defer conn.canWrite.L.Unlock()

	for len(data) > 0 {
		if err := conn.writeCheck(); err != nil {
			return totalWritten, err
		}

		lwipMutex.Lock()
		toWrite := len(data)
		if toWrite > int(conn.pcb.snd_buf) {
			// Write at most the size of the LWIP buffer.
			toWrite = int(conn.pcb.snd_buf)
		}
		if toWrite > 0 {
			written, err := conn.writeInternal(data[0:toWrite])
			totalWritten += written
			if err != nil {
				lwipMutex.Unlock()
				return totalWritten, err
			}
			data = data[written:]
		}
		lwipMutex.Unlock()
		if len(data) == 0 {
			break // Don't block if all the data has been written.
		}
		conn.canWrite.Wait()
	}

	return totalWritten, nil
}

func (conn *tcpConnEx) CloseWrite() error {
	conn.Lock()
	if conn.state >= tcpClosing || conn.state == tcpWriteClosed {
		conn.Unlock()
		return nil
	}
	if conn.state == tcpReceiveClosed {
		conn.state = tcpClosing
	} else {
		conn.state = tcpWriteClosed
	}
	conn.Unlock()

	lwipMutex.Lock()
	// FIXME Handle tcp_shutdown error.
	C.tcp_shutdown(conn.pcb, 0, 1)
	lwipMutex.Unlock()

	return nil
}

func (conn *tcpConnEx) CloseRead() error {
	return conn.patch.CloseReadPipe()
}

func (conn *tcpConnEx) Sent(len uint16) error {
	// Some packets are acknowledged by local client, check if any pending data to send.
	return conn.checkState()
}

func (conn *tcpConnEx) checkClosing() error {
	conn.Lock()
	defer conn.Unlock()

	if conn.state == tcpClosing {
		conn.closeInternal()
		return NewLWIPError(LWIP_ERR_OK)
	}
	return nil
}

func (conn *tcpConnEx) checkAborting() error {
	conn.Lock()
	defer conn.Unlock()

	if conn.state == tcpAborting {
		conn.abortInternal()
		return NewLWIPError(LWIP_ERR_ABRT)
	}
	return nil
}

func (conn *tcpConnEx) isClosed() bool {
	conn.Lock()
	defer conn.Unlock()

	return conn.state == tcpClosed
}

func (conn *tcpConnEx) checkState() error {
	if conn.isClosed() {
		return nil
	}

	err := conn.checkClosing()
	if err != nil {
		return err
	}

	err = conn.checkAborting()
	if err != nil {
		return err
	}

	// Signal the writer to try writting.
	conn.canWrite.Broadcast()

	return NewLWIPError(LWIP_ERR_OK)
}

func (conn *tcpConnEx) Close() error {
	conn.closeOnce.Do(conn.close)
	return conn.closeErr
}

func (conn *tcpConnEx) close() {
	err := conn.CloseRead()
	if err != nil {
		conn.closeErr = err
	}
	err = conn.CloseWrite()
	if err != nil {
		conn.closeErr = err
	}
}

func (conn *tcpConnEx) setLocalClosed() error {
	conn.Lock()
	defer conn.Unlock()

	if conn.state >= tcpClosing || conn.state == tcpReceiveClosed {
		return nil
	}

	// Causes the read half of the pipe returns.
	if conn.patch != nil {
		conn.patch.CloseWritePipe()
	}

	if conn.state == tcpWriteClosed {
		conn.state = tcpClosing
	} else {
		conn.state = tcpReceiveClosed
	}
	conn.canWrite.Broadcast()
	return nil
}

// Never call this function outside of the lwIP thread.
func (conn *tcpConnEx) closeInternal() error {
	C.tcp_arg(conn.pcb, nil)
	C.tcp_recv(conn.pcb, nil)
	C.tcp_sent(conn.pcb, nil)
	C.tcp_err(conn.pcb, nil)
	C.tcp_poll(conn.pcb, nil, 0)

	conn.release()

	// FIXME Handle error.
	err := C.tcp_close(conn.pcb)
	if err == C.ERR_OK {
		return nil
	} else {
		return errors.New(fmt.Sprintf("close TCP connection failed, lwip error code %d", int(err)))
	}
}

// Never call this function outside of the lwIP thread since it calls
// tcp_abort() and in that case we must return ERR_ABRT to lwIP.
func (conn *tcpConnEx) abortInternal() {
	conn.release()
	C.tcp_abort(conn.pcb)
}

func (conn *tcpConnEx) Abort() {
	conn.Lock()
	// If it's in tcpErrored state, the pcb was already freed.
	if conn.state < tcpAborting {
		conn.state = tcpAborting
	}
	conn.Unlock()

	lwipMutex.Lock()
	conn.checkState()
	lwipMutex.Unlock()
}

func (conn *tcpConnEx) Err(err error) {
	conn.Lock()
	defer conn.Unlock()

	conn.release()
	conn.state = tcpErrored
	conn.canWrite.Broadcast()
}

func (conn *tcpConnEx) LocalClosed() error {
	conn.setLocalClosed()
	return conn.checkState()
}

func (conn *tcpConnEx) release() {
	if _, found := tcpConns.Load(conn.connKey); found {
		freeConnKeyArg(conn.connKeyArg)
		tcpConns.Delete(conn.connKey)
	}
	conn.patch.CloseWritePipe()
	conn.patch.CloseReadPipe()
	conn.state = tcpClosed
}

func (conn *tcpConnEx) Poll() error {
	return conn.checkState()
}
