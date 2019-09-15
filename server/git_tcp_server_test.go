package server

import (
	"log"
	"net"
	"strings"
	"testing"
	"time"
)
var srv *TcpServer
func init() {
	srv = NewTcpServer(":8004")
	go func() {
		err := srv.Run()
		if err != nil {
			log.Fatal(err)
		}
	}()
}

func TestTcpServer_Write(t *testing.T) {
	time.Sleep(500 * time.Millisecond)
	client, err := net.Dial("tcp", "localhost:8004")
	if err != nil {
		t.Fatal(err)
	}
	connectMessage := "connect|user\n"
	if _, err := client.Write([]byte(connectMessage)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	count := srv.Count()
	if count != 1 {
		t.Fatalf("connected client should be 1; got %d", count)
	}
	_, err = srv.Client("user")
	if err != nil {
		t.Fatal("should return a conn belongs to user1")
	}
	p := &Payload{
		Key: "user",
		Message: "ping",
	}
	if err := srv.Write(p); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1024)
	_, err = client.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	s := string(buf[:])
	if strings.TrimSpace(s) != "ping" {
		t.Fatalf("Expected message; ping. got %s", s)
	}
}