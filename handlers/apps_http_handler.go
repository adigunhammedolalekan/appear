package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/adigunhammedolalekan/paas/build"
	"github.com/adigunhammedolalekan/paas/repos"
	"github.com/adigunhammedolalekan/paas/server"
	"github.com/adigunhammedolalekan/paas/types"
	"github.com/gin-gonic/gin"
	"gopkg.in/src-d/go-git.v4"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var repoBuildPath = "/Users/user/mnt/build"

type AppsHandler struct {
	tcp           *server.TcpServer
	appRepo       repos.AppsRepository
	dockerService *build.DockerService
}

func NewAppsHandler(
	tcp *server.TcpServer,
	repo repos.AppsRepository,
	dockerService *build.DockerService) *AppsHandler {
	return &AppsHandler{tcp: tcp, appRepo: repo, dockerService: dockerService}
}

func (handler *AppsHandler) CreateAppHandler(ctx *gin.Context) {
	user := ParseToken(ctx)
	if user == nil {
		ctx.JSON(http.StatusUnauthorized, &Response{Error: true, Message: "token not found"})
		return
	}

	opt := &types.CreateAppOpts{}
	if err := ctx.ShouldBindJSON(opt); err != nil {
		ctx.JSON(http.StatusBadRequest, &Response{Error: true, Message: "bad request: malformed JSON body"})
		return
	}

	newApp, err := handler.appRepo.CreateApp(opt)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &Response{Error: true, Message: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, &Response{Error: false, Message: "app created", Data: newApp})
}

func (handler *AppsHandler) BuildAppHandler(ctx *gin.Context) {
	opt := &types.HookInfo{}
	if err := ctx.ShouldBindJSON(opt); err != nil {
		ctx.JSON(http.StatusBadRequest, nil)
		return
	}

	log.Println(opt)
	tcpPayload := &server.Payload{
		Key: opt.Key,
	}

	repoPath := handler.resolveRepoUrl(opt.RepoPath)
	repoUri := fmt.Sprintf("%s/%s", os.Getenv("REPO_SERVER_BASE_URL"), repoPath)
	log.Println(repoUri)
	app, err := handler.appRepo.GetAppByRepositoryUrl(repoUri)
	if err != nil {
		tcpPayload.Message = "404 app not found: failed to find app"
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true, Message: err.Error()})
		return
	}

	clonePath := filepath.Join(repoBuildPath, opt.RepoName)
	commit, err := handler.appRepo.CloneRepository(handler.resolveRepoUsername(repoPath), clonePath, repoUri)
	if err != nil && err != git.ErrRepositoryAlreadyExists {
		tcpPayload.Message = "failed to build repo " + err.Error()
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true, Message: err.Error()})
		return
	}

	config, err := handler.readConfigFromRepo(clonePath)
	if err != nil {
		tcpPayload.Message = "failed to read PaaS config. " + err.Error()
		handler.writeTcpMessage(tcpPayload)
	}

	user := commit.Author.Email
	tcpPayload.Message = "calling user is " + user
	handler.writeTcpMessage(tcpPayload)
	result, err := handler.dockerService.BuildLocalImage(clonePath, build.CreateBuildFromConfig(config))
	if err != nil {
		tcpPayload.Message = "failed to build image " + err.Error()
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true})
		return
	}
	type buildMessage struct {
		Stream string `json:"stream"`
	}
	for m := range result.Log {
		s := &buildMessage{}
		if err := json.Unmarshal([]byte(m), s); err != nil {
			log.Println("json error ", err)
			s.Stream = m
		}
		tcpPayload.Message = s.Stream
		handler.writeTcpMessage(tcpPayload)
	}

	app.ImageName = result.PullPath
	if err := handler.appRepo.UpdateDeployment(app); err != nil {
		tcpPayload.Message = "failed to update deployment " + err.Error()
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true, Message: err.Error()})
		return
	}

	if err := handler.appRepo.LogDeploymentEvent(user, app.ID); err != nil {
		tcpPayload.Message = "failed to log deployment event"
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true})
		return
	}
	ctx.JSON(http.StatusOK, nil)
}

func (handler *AppsHandler) resolveRepoUrl(s string) string {
	log.Println("resolving repo path ", s)
	parts := strings.Split(s, "/")
	lastIdx := len(parts) - 1
	idx := lastIdx - 1
	return fmt.Sprintf("%s/%s", parts[idx], parts[lastIdx])
}

func (handler *AppsHandler) resolveRepoUsername(s string) string {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

func (handler *AppsHandler) writeTcpMessage(p *server.Payload) {
	log.Println("[BuildApp]: ", p.Message)
	if err := handler.tcp.Write(p); err != nil {
		log.Println("[TCP]: failed to write message to TCP client ", err)
	}
}

func (handler *AppsHandler) readConfigFromRepo(path string) (*build.Config, error) {
	fullpath := filepath.Join(path, "paas_config.json")
	data, err := ioutil.ReadFile(fullpath)
	if err != nil {
		return &build.Config{}, errors.New("paas_config.json is missing")
	}
	cfg := &build.Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return &build.Config{}, errors.New("failed to read paas_config.json. malformed json data")
	}
	return cfg, nil
}

type Response struct {
	Error   bool        `json:"error"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}
