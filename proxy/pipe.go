package proxy

import (
	"encoding/json"
	"io"
	"log"
	"time"
)

// ToPipe 发送数据到pipe
func ToPipe(serversock Sock, r role) {
	defer serversock.Close()
	p := make([]byte, pipeMaxContentLength)

	pipesock := GetPipeSock(serversock)
	if pipesock == nil {
		// 未找到pipe连接，直接关闭server连接
		log.Printf("pipe is not running")
		return
	}

	// 拿到当前连接的id和请求id
	serversockConfig := serversock.Config()
	id, reqID := serversockConfig.ID, serversockConfig.ReqID

	writer := NewBufWriter(pipesock)

	// 当首次建立连接的时候要向client实例发送一个打开[client实例到访问目标]的连接，保证连接数量是对等的
	// 只有server实例可以向pipe发送打开连接的请求
	if r == RoleServer {
		// 通知连接开启
		log.Printf("write to pipe:open, id=%d, reqID=%d", id, reqID)
		_, err := writer.WriteHeader(id, reqID, Open)
		if err != nil {
			pipesock.Close()
			log.Printf(">>pipe:error id=%d, reqID=%d, err=%s", id, reqID, err.Error())
			return
		}
	}

	for {
		n, err := serversock.Read(p)
		log.Printf("read from server:id=%d, reqID=%d, n=%d, err=%s", id, reqID, n, err)
		var werr error
		if n > 0 {
			log.Printf("write to pipe:success id=%d, reqID=%d, n=%d, err=%s", id, reqID, n, err)
			_, werr = writer.WriteContent(id, reqID, Success, p[:n])
		}
		if err != nil {
			// log.Printf(" id=%d, reqID=%d, err=%s", id, reqID, err.Error())
			if err == io.EOF {
				log.Printf("write to pipe:closed id=%d, reqID=%d, err=%s", id, reqID, err)
				writer.WriteHeader(id, reqID, Closed)
			}
			log.Printf("server close: id=%d, reqID=%d, err=%s", id, reqID, err)
			break
		}

		if werr != nil {
			log.Printf(">>pipe:error id=%d, reqID=%d, err=%s", id, reqID, err.Error())
			pipesock.Close()
			break
		}
	}
}

// ToServer 发送数据到server
func ToServer(pipesock Sock, r role) {

	p := make([]byte, pipeMaxContentLength)

	reader := NewBufReader(pipesock)
	writer := NewBufWriter(pipesock)
	defer func() {
		pipesock.Close()
		// 当role=RoleClient的pipe连接丢失的时候，尝试重连
		if r == RoleClient {
			pipeClosedChan <- pipesock
		}
	}()

	// client每次创建pipe连接发送当前server配置，并配置当前的连接
	if r == RoleClient {
		sockconfig := pipesock.Config()

		hsConfs := []*ConfigSingle{}
		for _, v := range Configs.Server {
			if v.PID == sockconfig.ID {
				hsConfs = append(hsConfs, v)
			}
		}
		confBytes, _ := json.Marshal(hsConfs)
		_, err := writer.WriteContent(0, 0, HandShake, confBytes)
		if err != nil {
			return
		}
		sockconfig.ServerID = []int{}
		for _, v := range hsConfs {
			sockconfig.ServerID = append(sockconfig.ServerID, v.ID)
		}
	}
	for {
		n, err := reader.Read(p)
		log.Printf("read from pipe:id=%d, reqID=%d, n=%d, status=%d, err=%s", reader.h.ID, reader.h.ReqID, n, reader.h.Status, err)

		switch reader.h.Status {
		case Heartbeat:
			continue
		case HandShake:
			sockconfig := pipesock.Config()
			sockconfig.ServerID = []int{}
			_serverConf := []*ConfigSingle{}
			if err = json.Unmarshal(p[:n], &_serverConf); err != nil {
				log.Printf("handshakeError:%s", err.Error())
				break
			}
			for _, v := range _serverConf {
				sockconfig.ServerID = append(sockconfig.ServerID, v.ID)
			}
			continue
		}

		var serversock Sock

		// pipe接收到打开连接的通知
		if reader.h.Status == Open {
			log.Printf("new sock:id=%d, reqID=%d", reader.h.ID, reader.h.ReqID)
			serversock, _ = NewSock(int(reader.h.ID), int(reader.h.ReqID), TypeServer)
		} else {
			serversock = GetServerSock(int(reader.h.ID), int(reader.h.ReqID))
		}

		if serversock == nil {
			log.Printf("no sock founded:id=%d, reqID=%d, status=%d", reader.h.ID, reader.h.ReqID, reader.h.Status)
			if err != nil {
				break
			}

			if reader.h.Status != Closed {
				log.Printf("write to pipe:closed id=%d, reqID=%d, status now =%d", reader.h.ID, reader.h.ReqID, reader.h.Status)
				_, err := writer.WriteHeader(int(reader.h.ID), int(reader.h.ReqID), Closed)
				if err != nil {
					break
				}
			}

			continue
		}

		var werr error
		if n > 0 {
			_, werr = serversock.Write(p[:n])
			log.Printf("write to server: id=%d, reqID=%d, status=%d n=%d, err=%s", reader.h.ID, reader.h.ReqID, reader.h.Status, n, werr)
		}
		if reader.h.Status == Closed {
			log.Printf("closeing sock")
			serversock.Close()
			continue
		}

		if err != nil {
			log.Printf("closeing sock: id=%d, reqID=%d, status=%d, err=%s", reader.h.ID, reader.h.ReqID, reader.h.Status, err.Error())
			serversock.Close()
			break
		}

		if werr != nil {
			writer.WriteHeader(int(reader.h.ID), int(reader.h.ReqID), Closed)
			serversock.Close()
			continue
		}
	}
}

// heartbeat 针对pipe连接的心跳检测，60秒一次
func heartbeat(pipesock Sock, r role) {
	for {
		select {
		case <-time.After(time.Second * 60):
			writer := NewBufWriter(pipesock)
			_, err := writer.WriteHeader(0, 0, Heartbeat)
			if err != nil {
				pipesock.Close()
				// 当role=RoleClient的pipe连接丢失的时候，尝试重连
				if r == RoleClient {
					pipeClosedChan <- pipesock
				}
				return
			}
		}
	}

}

type role int

// RoleServer 表示了server实例所扮演的角色
const RoleServer role = 1

// RoleClient 表示client实例在包中的角色
const RoleClient role = 2

// DialSocks 开始读取chan中接收到的新建立的连接的数据，并转发到对应的pipe或server中
// 调用时需传入当前实例的角色常量
func DialSocks(r role) {
	for {
		select {
		case sock := <-sockChan:
			sockConfig := sock.Config()

			// 新建立的连接在这里加入连接池，而不是在连接建立成功后
			socks.addsock(sockConfig.Type, sockConfig.ID, sock)

			// 当连接类型为pipe连接时，执行ToServer方法把读取到的数据发送到server端
			// 如果类型为server连接，执行ToPipe把从server端读取的数据加入header后发送到pipe
			// 新建立的pipe连接，client应该向server发送一个标识了改连接所支持的serverid的数据
			if sockConfig.Type == TypePipe {
				go ToServer(sock, r)
				go heartbeat(sock, r)
			} else if sockConfig.Type == TypeServer {
				go ToPipe(sock, r)
			}
		case sock := <-pipeClosedChan:
			sockConfig := sock.Config()
			time.Sleep(time.Second * 3)
			_, err := NewSock(sockConfig.ID, 0, TypePipe)
			if err != nil {
				pipeClosedChan <- sock
			}
		}
	}
}
