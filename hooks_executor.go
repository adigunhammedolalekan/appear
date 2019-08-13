package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func __main() {
	out := os.Stdout
	info, err := ReadHookInput(os.Stdin)
	if err != nil {
		out.Write([]byte("failed to read hook"))
		return
	}

	info.Key = "key" + randString()[:5]
	u := "http://paas:8007/api/build"
	j, _ := json.Marshal(info)
	out.Write([]byte("Building app..."))
	req, err := http.NewRequest("POST", u, bytes.NewReader(j))
	if err != nil {
		out.Write([]byte("failed to build request"))
		return
	}

	t := newTcpClient(info.Key)
	t.write(info.Key)
	go func() {
		t.start(out)
	}()

	for _, pair := range os.Environ() {
		out.Write([]byte(pair))
		out.Write([]byte("\n"))
	}

	c := &http.Client{}
	resp, err := c.Do(req)
	if err != nil {
		out.Write([]byte("error response from API " + err.Error()))
		return
	}

	out.Write([]byte("Built app " + resp.Status))
}

type HookInfo struct {
	RepoName string `json:"repo_name"`
	RepoPath string `json:"repo_path"`
	OldRev   string `json:"old_rev"`
	NewRev   string `json:"new_rev"`
	Ref      string `json:"ref"`
	RefType  string `json:"ref_type"`
	RefName  string `json:"ref_name"`
	Key      string `json:"key"`
}

func ReadHookInput(input io.Reader) (*HookInfo, error) {
	reader := bufio.NewReader(input)

	line, _, err := reader.ReadLine()
	if err != nil {
		return nil, err
	}

	chunks := strings.Split(string(line), " ")
	if len(chunks) != 3 {
		return nil, fmt.Errorf("Invalid hook input")
	}
	refchunks := strings.Split(chunks[2], "/")

	dir, _ := os.Getwd()
	info := HookInfo{
		RepoName: filepath.Base(dir),
		RepoPath: dir,
		OldRev:   chunks[0],
		NewRev:   chunks[1],
		Ref:      chunks[2],
		RefType:  refchunks[1],
		RefName:  refchunks[2],
	}

	return &info, nil
}

type TcpClient struct {
	conn net.Conn
}

func newTcpClient(key string) *TcpClient {
	conn, err := net.Dial("tcp", "paas:9010")
	if err != nil {

	}
	return &TcpClient{conn: conn}
}

func (t *TcpClient) start(fi *os.File) {
	for {
		buf := make([]byte, 512)
		_, err := t.conn.Read(buf)
		if err != nil && err != io.EOF {
			break
		}

		fi.Write(buf[:])
	}
}

func (t *TcpClient) write(key string) {
	m := "connect|" + key
	_, err := t.conn.Write([]byte(m))
	if err != nil {
	}
}

func randString() string {
	return GenerateRandomString(10)
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func init() { rand.Seed(time.Now().UnixNano()) }

// GenerateRandomString returns a randomly generated string
func GenerateRandomString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
