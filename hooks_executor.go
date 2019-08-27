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

const tcpServerAddr = "localhost:9010"
const appBuildUrl = "http://localhost:9060/api/build"

func main() {
	out := os.Stdout
	info, err := ReadHookInput(os.Stdin)
	if err != nil {
		out.Write([]byte("failed to read hook"))
		return
	}

	info.Key = "key" + randString()[:5]
	j, _ := json.Marshal(info)
	out.Write([]byte("Building app..."))
	req, err := http.NewRequest("POST", appBuildUrl, bytes.NewReader(j))
	if err != nil {
		out.Write([]byte("failed to docker request"))
		return
	}

	t, err := newTcpClient()
	if err != nil {
		out.Write([]byte("failed to docker app. Internal TCP server error " + err.Error()))
		return
	}

	if err := t.write(strings.TrimSpace(info.Key)); err != nil {
		out.Write([]byte("failed to register client " + err.Error()))
	}

	go func() {
		t.start(out)
	}()

	c := &http.Client{}
	resp, err := c.Do(req)
	if err != nil {
		out.Write([]byte("error response from API " + err.Error()))
		return
	}
	type R struct {
		Error   bool   `json:"error"`
		Message string `json:"message"`
	}
	r := &R{}
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(r); err != nil {
		out.Write([]byte("failed to decode response " + err.Error()))
		return
	}
	message := ""
	if r.Error {
		message = "failed to docker app: " + r.Message
	} else {
		message = "app built: " + message
	}
	out.Write([]byte(message))
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

func newTcpClient() (*TcpClient, error) {
	conn, err := net.Dial("tcp", tcpServerAddr)
	if err != nil {
		return nil, err
	}
	return &TcpClient{conn: conn}, nil
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

func (t *TcpClient) write(key string) error {
	m := fmt.Sprintf("connect|%s\n", key)
	_, err := t.conn.Write([]byte(m))
	if err != nil {
		return err
	}
	return nil
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
