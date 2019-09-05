package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/adigunhammedolalekan/paas/docker"
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

type AppsHandler struct {
	tcp           *server.TcpServer
	appRepo       repos.AppsRepository
	dockerService *docker.DockerService
	repoBuildPath string
}

func NewAppsHandler(
	tcp *server.TcpServer,
	repo repos.AppsRepository,
	dockerService *docker.DockerService,
	repoBuildPath string) *AppsHandler {
	return &AppsHandler{tcp: tcp, appRepo: repo, dockerService: dockerService, repoBuildPath: repoBuildPath}
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
	opt.UserId = user.ID
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

	repoPath := handler.resolveRepoUrl(opt.RepoPath)
	repoUri := fmt.Sprintf("%s/%s", os.Getenv("REPO_SERVER_BASE_URL"), repoPath)
	app, err := handler.appRepo.GetAppByRepositoryUrl(repoUri)
	if err != nil {
		tcpPayload.Message = "404: app not found"
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true, Message: err.Error()})
		return
	}

	clonePath := filepath.Join(handler.repoBuildPath, opt.RepoName)
	commit, err := handler.appRepo.CloneRepository(handler.resolveRepoUsername(repoPath), clonePath, repoUri)
	if err != nil && err != git.ErrRepositoryAlreadyExists {
		tcpPayload.Message = "failed to clone repo: " + err.Error()
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true, Message: err.Error()})
		return
	}

	config, err := handler.readConfigFromRepo(app.Name, clonePath)
	if err != nil {
		tcpPayload.Message = "failed to read config from repository: " + err.Error()
		handler.writeTcpMessage(tcpPayload)
	}

	user := commit.Author.Email
	// handler.writeTcpMessage(tcpPayload)
	result, err := handler.dockerService.BuildLocalImage(clonePath, docker.CreateBuildFromConfig(config))
	if err != nil {
		tcpPayload.Message = "failed to build docker image: " + err.Error()
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true})
		return
	} else {
		tcpPayload.Message = "configuration file detected"
		handler.writeTcpMessage(tcpPayload)
	}
	type buildMessage struct {
		Stream string `json:"stream"`
		Status string `json:"status"`
	}
	for m := range result.Log {
		s := &buildMessage{}
		if err := json.Unmarshal([]byte(handler.validUtf8String(m)), s); err != nil {
			log.Println("json error: ", err)
			s.Stream = m
		}
		message := s.Status
		if message == "" {
			message = s.Stream
		}
		tcpPayload.Message = message
		handler.writeTcpMessage(tcpPayload)
	}

	pushChan, err := handler.dockerService.PushImage(result.PullPath)
	if err != nil {
		tcpPayload.Message = "failed to push image: " + err.Error()
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true, Message: err.Error()})
		return
	}
	for r := range pushChan {
		tcpPayload.Message = r
		handler.writeTcpMessage(tcpPayload)
	}

	app.ImageName = result.PullPath
	if err := handler.appRepo.UpdateDeployment(app); err != nil {
		tcpPayload.Message = "failed to update deployment: " + err.Error()
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true, Message: err.Error()})
		return
	}

	if err := handler.appRepo.LogDeploymentEvent(user, app.ID); err != nil {
		tcpPayload.Message = "failed to log deployment event"
		handler.writeTcpMessage(tcpPayload)
		ctx.JSON(http.StatusOK, &Response{Error: true, Message: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, nil)
}

func (handler *AppsHandler) LogsHandler(ctx *gin.Context) {
	name := ctx.Query("name")
	if name == "" {
		ctx.JSON(http.StatusBadRequest, &Response{Error: true, Message: "bad request: app name is missing"})
		return
	}
	s, err := handler.appRepo.Logs(name)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &Response{Error: true, Message: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, &Response{Error: false, Message: s})
}

func (handler *AppsHandler) ProvisionDbHandler(ctx *gin.Context) {
	opt := &types.ProvisionDatabaseRequest{}
	if err := ctx.ShouldBindJSON(opt); err != nil {
		ctx.JSON(http.StatusBadRequest, &Response{Error: true, Message: "bad request: malformed json body"})
		return
	}
	result, err := handler.appRepo.ProvisionDatabase(opt)
	if err != nil {
		log.Println("failed to provision database: ", err)
		ctx.JSON(http.StatusInternalServerError, &Response{Error: true, Message: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, &Response{Error: false, Message: "success", Data: result})
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

func (handler *AppsHandler) readConfigFromRepo(appName, path string) (*docker.Config, error) {
	fullpath := filepath.Join(path, "paas_config.json")
	data, err := ioutil.ReadFile(fullpath)
	defaultConfig := &docker.Config{Name: appName}
	if err != nil {
		return defaultConfig, errors.New("paas_config.json is missing")
	}
	cfg := &docker.Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return defaultConfig, errors.New("failed to read paas_config.json. malformed json data")
	}
	if cfg.Name == "" {
		cfg.Name = appName
	}
	return cfg, nil
}

func (handler *AppsHandler) validUtf8String(s string) string {
	input := strings.ReplaceAll(s, "\u003e", "")
	newString := strings.ReplaceAll(input, "\n", "")
	log.Println(newString)
	return newString
}

type Response struct {
	Error   bool        `json:"error"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}
