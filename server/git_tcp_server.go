package server

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
)

type TcpServer struct {
	conns map[string]net.Conn
	mtx   sync.RWMutex
	addr  string
}

type ConnMessage struct {
	Action string
	Key    string
}

func (c *ConnMessage) KeyString() string {
	return strings.TrimSpace(c.Key)
}

func (c *ConnMessage) ActionString() string {
	return strings.TrimSpace(c.Action)
}

type Payload struct {
	Key     string
	Message string
}

func (p *Payload) KeyString() string {
	return strings.TrimSpace(p.Key)
}

func NewTcpServer(addr string) *TcpServer {
	s := &TcpServer{}
	s.conns = make(map[string]net.Conn)
	s.addr = addr
	return s
}

func (s *TcpServer) Run() error {
	handle, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	for {
		conn, err := handle.Accept()
		if err != nil {
			log.Println("failed to accept remote conn: ", err)
			continue
		}
		go s.handleConn(conn)
	}
}

func (s *TcpServer) handleConn(conn net.Conn) {
	for {
		buf := bufio.NewReader(conn)

		m, err := buf.ReadString(byte('\n'))
		if err != nil || err == io.EOF {
			log.Println("[TCP]: error reading from client ", err)
			break
		}

		message := s.parseMessage(strings.TrimSpace(m))
		if message != nil {
			if message.ActionString() == "connect" {
				s.register(message, conn)
				break
			}
		}
	}
}

func (s *TcpServer) register(m *ConnMessage, conn net.Conn) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	log.Println("[TCP]: registering connection ", m.Key)
	s.conns[m.KeyString()] = conn
}

func (s *TcpServer) Write(p *Payload) error {
	s.mtx.Lock()
	conn, ok := s.conns[p.KeyString()]
	s.mtx.Unlock()

	if !ok {
		return fmt.Errorf("net.Conn not found for key %s", p.KeyString())
	}

	n, err := conn.Write([]byte(fmt.Sprintf("%s\n", p.Message)))
	if err != nil {
		return err
	}
	log.Println("written: ", n, " to client: ", p.Key)
	return nil
}

func (s *TcpServer) parseMessage(m string) *ConnMessage {
	log.Println("parsing message ", m)
	parts := strings.Split(m, "|")
	if len(parts) != 2 {
		return nil
	}

	return &ConnMessage{Action: parts[0], Key: parts[1]}
}