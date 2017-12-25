package proxy

import (
	"errors"
	"log"
	"net"
	"strconv"
)

// TCPListener TCPListener
type TCPListener struct {
	Listener net.Listener
	ID       int
	PID      int
	Type     int
}

// NewTCPListener 创建一个tcp监听
func NewTCPListener(config *ConfigSingle) (tcp *TCPListener, err error) {
	ln, err := net.Listen(config.Protocol, config.IP+":"+strconv.Itoa(config.Port))
	if err != nil {
		return
	}

	log.Printf("listening %s://%s:%d\n", config.Protocol, config.IP, config.Port)

	return &TCPListener{ln, config.ID, config.PID, config.Type}, nil
}

// Accept accept
func (tcp *TCPListener) Accept() {
	reqID := 0
	for {
		conn, err := tcp.Listener.Accept()
		if err != nil {
			log.Println("tcp accept error:", err.Error())
			continue
		}
		reqID++
		log.Printf("accept %s, reqID=%d\n", tcp.Listener.Addr(), reqID)
		sockConfig := &SockConfig{tcp.ID, reqID, tcp.PID, tcp.Type, []int{}}
		sock := &TCPSock{conn, sockConfig}
		sockChan <- sock
		// socks.addsock(tcp.Type, tcp.ID, sock)
	}
}

// TCPSock 实现了Sock接口
type TCPSock struct {
	Conn net.Conn
	*SockConfig
}

func (t *TCPSock) Write(p []byte) (n int, err error) {
	n, err = t.Conn.Write(p)
	return
}

func (t *TCPSock) Read(p []byte) (n int, err error) {
	n, err = t.Conn.Read(p)
	return
}

func (t *TCPSock) Config() (conf *SockConfig) {
	return t.SockConfig
}

func (s *TCPSock) Close() (err error) {
	// 从连接池里移除连接
	err = socks.delsock(s.Type, s.ID, s)
	if err == nil {
		log.Println("closing", s.ReqID)
		return s.Conn.Close()
	}
	return errors.New("can not find TCPSock on close")
}

// NewTCPClient 创建一个client客户端
func NewTCPClient(config *ConfigSingle, reqID int) (sock *TCPSock, err error) {
	conn, err := net.Dial(config.Protocol, config.IP+":"+strconv.Itoa(config.Port))
	if err != nil {
		return
	}
	log.Printf("connect to server %s, reqID = %d", conn.RemoteAddr(), reqID)
	sockConfig := &SockConfig{config.ID, reqID, config.PID, config.Type, []int{}}
	sock = &TCPSock{conn, sockConfig}
	sockChan <- sock
	// socks.addsock(config.Type, config.ID, sock)
	return
}
