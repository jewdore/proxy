package proxy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math/rand"
	"sync"
	"time"
)

const (
	pipeMaxContentLength = 2048
)

const (
	// Open 当server端打开一个连接，需要在client端也打开一个连接
	Open = iota + 1

	// Success 成功接收数据
	Success

	// Closed 连接被关闭
	Closed

	// Heartbeat 心跳
	Heartbeat

	// HandShake 表示pipe创建连接时的数据互换初始化
	// 1. 填充client端server配置的所有serverid
	HandShake
)

// 连接池
type socksPool struct {
	Socks map[int]map[int][]Sock
	Lock  sync.Mutex
}

var (
	padding        = [255]byte{}
	socks          = &socksPool{}
	sockChan       chan Sock // 新建立的连接的管道
	pipeClosedChan chan Sock // pipe连接丢失重连的管道
)

func init() {
	socks.Socks = map[int]map[int][]Sock{}
	sockChan = make(chan Sock, 200)
	pipeClosedChan = make(chan Sock, 200)
}

func (s *socksPool) addsock(socktype, id int, sock Sock) {
	s.Lock.Lock()
	defer s.Lock.Unlock()
	if _, ok := s.Socks[socktype]; !ok {
		s.Socks[socktype] = map[int][]Sock{}
	}

	if _, ok := s.Socks[socktype][id]; !ok {
		s.Socks[socktype][id] = []Sock{}
	}
	s.Socks[socktype][id] = append(s.Socks[socktype][id], sock)
}

func (s *socksPool) delsock(socktype, id int, sock Sock) (err error) {
	s.Lock.Lock()
	defer s.Lock.Unlock()
	if s.Socks[socktype][id] == nil {
		return errors.New("no item")
	}
	for k, _sock := range s.Socks[socktype][id] {
		if _sock == sock {
			s.Socks[socktype][id] = append(s.Socks[socktype][id][:k], s.Socks[socktype][id][k+1:]...)
			return nil
		}
	}
	return errors.New("no item")
}

func (s *socksPool) getSockMap(socktype int) map[int][]Sock {
	s.Lock.Lock()
	defer s.Lock.Unlock()
	return s.Socks[socktype]
}

type header struct {
	ID            uint8
	ReqID         uint16
	BodyLength    uint16
	PaddingLength uint8
	Status        uint8
}

func (h *header) Init(id, reqID, contentLength, status int) {
	h.BodyLength = uint16(contentLength)
	h.PaddingLength = uint8((-contentLength & 7))
	h.ID = uint8(id)
	h.ReqID = uint16(reqID)
	h.Status = uint8(status)
}

// Sock 接口将会在pipe，server的监听及读写中作为通用参数
type Sock interface {
	io.ReadWriteCloser
	Config() (conf *SockConfig)
}

type SockConfig struct {
	ID       int
	ReqID    int
	PID      int
	Type     int
	ServerID []int // 如果连接是pipe类型，应该填充该连接支持的client端的serverID
}

// BufWriter 主要用在对pipe的写数据，以实现带header的通信机制
type BufWriter struct {
	sock Sock
	buf  bytes.Buffer
	h    header
	lock sync.Mutex
}

func NewBufWriter(sock Sock) (bw *BufWriter) {
	bw = &BufWriter{}
	bw.sock = sock
	return
}

// WriteHeader 单独写一个header
func (bw *BufWriter) WriteHeader(id, reqID, status int) (n int, err error) {
	return bw.WriteContent(id, reqID, status, []byte{})
}

// WriteContent 写一条带有header的数据
func (bw *BufWriter) WriteContent(id, reqID, status int, p []byte) (n int, err error) {
	bw.lock.Lock()
	defer bw.lock.Unlock()
	bw.buf.Reset()
	bw.h.Init(id, reqID, len(p), status)
	if err := binary.Write(&bw.buf, binary.BigEndian, bw.h); err != nil {
		return 0, err
	}
	if _, err := bw.buf.Write(p); err != nil {
		return 0, err
	}
	if _, err := bw.buf.Write(padding[:bw.h.PaddingLength]); err != nil {
		return 0, err
	}
	return bw.sock.Write(bw.buf.Bytes())
}

// BufReader 提供从pipe接收带有header的数据的读取方式
type BufReader struct {
	sock Sock
	buf  []byte
	h    header
}

func NewBufReader(r Sock) (br *BufReader) {
	br = &BufReader{}
	br.sock = r
	return
}

func (sr *BufReader) Read(p []byte) (n int, err error) {
	sr.h = header{}
	sr.buf = []byte{}
	if err := binary.Read(sr.sock, binary.BigEndian, &sr.h); err != nil {
		return 0, err
	}
	sr.buf = make([]byte, int(sr.h.BodyLength)+int(sr.h.PaddingLength))
	n, err = io.ReadFull(sr.sock, sr.buf)
	sr.buf = sr.buf[:n]
	copy(p, sr.buf[:int(sr.h.BodyLength)])
	return int(sr.h.BodyLength), err
}

// GetPipeSock 从连接池获取pipe连接
// 需要一个serversock，将会从serversock的属性pid对应的连接中随机抽取一个pipe的连接
func GetPipeSock(serversock Sock) (s Sock) {
	serversockConfig := serversock.Config()
	pipesocks := socks.getSockMap(TypePipe)[serversockConfig.PID]
	if pipesocks == nil {
		return nil
	}

	// 过滤出当前server可以使用的pipe连接
	_socks := []Sock{}
	for _, v := range pipesocks {
		pipeconf := v.Config()
		for _, sid := range pipeconf.ServerID {
			if sid == serversockConfig.ID {
				_socks = append(_socks, v)
				break
			}
		}
	}

	if len(_socks) == 0 {
		return nil
	}
	if len(_socks) == 1 {
		return _socks[0]
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return _socks[r.Intn(len(_socks)-1)]
}

// GetServerSock 从server连接池里获取连接
func GetServerSock(id, reqID int) (serversock Sock) {
	serversocks := socks.getSockMap(TypeServer)[id]
	if serversocks == nil {
		return nil
	}
	for _, sock := range serversocks {
		sockconfig := sock.Config()
		if sockconfig.ReqID == reqID {
			return sock
		}
	}

	return nil
}

// NewSock 手动新建一个[pipe >> 访问目标主机端口]连接
func NewSock(id, reqID, sockType int) (sock Sock, err error) {
	var config *ConfigSingle
	if sockType == TypePipe {
		config = Configs.getPipe(id)
	} else {
		config = Configs.getServer(id)
	}

	if config == nil {
		return nil, errors.New("config error")
	}

	if config.Protocol == "tcp" {
		return NewTCPClient(config, reqID)
	}
	return nil, errors.New("config error")
}
