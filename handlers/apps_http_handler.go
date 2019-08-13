package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/adigunhammedolalekan/paas/build"
	"github.com/adigunhammedolalekan/paas/repos"
	"github.com/adigunhammedolalekan/paas/server"
	"github.com/adigunhammedolalekan/paas/types"
	"github.com/gin-gonic/gin"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var repoBuildPath = "/var/build"

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

func (handler *AppsHandler) CreateApp(ctx *gin.Context) {
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

	tcpPayload := &server.Payload{
		Key: opt.Key,
	}

	repoUri := fmt.Sprintf("%s%s", os.Getenv("REPO_SERVER_BASE_URL"), opt.RepoName)
	app, err := handler.appRepo.GetAppByRepositoryUrl(repoUri)
	if err != nil {
		tcpPayload.Message = "404 app not found: failed to find app"
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true, Message: err.Error()})
		return
	}

	clonePath := filepath.Join(repoBuildPath, opt.RepoName)
	commit, err := handler.appRepo.CloneRepository(clonePath, repoUri)
	if err != nil {
		tcpPayload.Message = "failed to build repo " + err.Error()
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true, Message: err.Error()})
		return
	}

	config, err := handler.readConfigFromRepo(clonePath)
	if err != nil {
		tcpPayload.Message = "failed to read PaaS config"
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true})
		return
	}

	user := commit.Author.Email
	result, err := handler.dockerService.BuildLocalImage(clonePath, build.CreateBuildFromConfig(config))
	if err != nil {
		tcpPayload.Message = "failed to build image " + err.Error()
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true})
		return
	}
	for m := range result.Log {
		tcpPayload.Message = m
		handler.writeTcpMessage(tcpPayload)
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
	parts := strings.Split(s, "/")
	if len(parts) != 4 {
		return ""
	}
	return "http://paas:9798/" + parts[2] + "/" + parts[3]
}

func (handler *AppsHandler) writeTcpMessage(p *server.Payload) {
	if err := handler.tcp.Write(p); err != nil {
		log.Println("[TCP]: failed to write message to TCP client ", err)
	}
}

func (handler *AppsHandler) readConfigFromRepo(path string) (*build.Config, error) {
	fullpath := filepath.Join(path, "paas_config.json")
	data, err := ioutil.ReadFile(fullpath)
	if err != nil {
		return nil, err
	}
	cfg := &build.Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

type Response struct {
	Error   bool        `json:"error"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}
