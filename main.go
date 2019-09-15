package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/adigunhammedolalekan/paas/config"
	"github.com/adigunhammedolalekan/paas/docker"
	"github.com/adigunhammedolalekan/paas/git"
	"github.com/adigunhammedolalekan/paas/handlers"
	"github.com/adigunhammedolalekan/paas/k8s"
	"github.com/adigunhammedolalekan/paas/repos"
	"github.com/adigunhammedolalekan/paas/server"
	"github.com/adigunhammedolalekan/paas/types"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("failed to load environment variables")
	}
	args := os.Args
	if len(args) > 1 {
		arg := args[1]
		if arg == "init" {
			if err := config.InitDefaultConfig(); err != nil {
				log.Fatal("Failed to init project config: ", err)
			} else {
				log.Println("initialization successful")
				os.Exit(0)
			}
		}
	}

	var configFilePath string
	flag.StringVar(&configFilePath, "config", "", "appear configuration file path")
	flag.Parse()
	if configFilePath == "" {
		wd, err := os.Getwd()
		if err != nil {
			// do what?
			wd = ""
		}
		path := filepath.Join(wd, "appear_config.json")
		log.Printf("Configuration file path is missing. Defaulting to %s", path)
		configFilePath = path
	}
	sv := NewServer(configFilePath)
	addr := fmt.Sprintf(":%s", os.Getenv("PORT"))
	if err := sv.Run(addr); err != nil {
		log.Fatal("Failed to start appear server: ", err)
	}
}

type Server struct {
	Router    *gin.Engine
	configDir string
	config    *config.Configuration
}

func NewServer(configDir string) *Server {
	s := &Server{}
	gin.SetMode(gin.ReleaseMode)
	s.Router = gin.Default()
	s.configDir = configDir
	return s
}

func (s *Server) Run(addr string) error {
	if err := s.parseConfig(); err != nil {
		return err
	}
	router := s.Router
	db, err := s.createDbConnection(s.config.DatabaseUrl)
	if err != nil {
		return err
	}
	service, err := git.NewService(db, s.config.GitStoragePath)
	if err != nil {
		return err
	}
	dockerClient, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	k8sService, err := k8s.NewK8sService(s.config.Registry)
	if err != nil {
		return err
	}

	dockerService := docker.NewDockerService(dockerClient, s.config.Registry)
	tcp := server.NewTcpServer(os.Getenv("TCP_SERVER_ADDR"))
	appRepo := repos.NewAppRepository(db, service, k8sService)
	accountRepo := repos.NewUserRepository(db)
	appHandler := handlers.NewAppsHandler(tcp, appRepo, dockerService, s.config.DockerBuildPath)
	accountHandler := handlers.NewUserHandler(accountRepo)
	authKey := s.config.Auth.MasterAuthKey
	if authKey == "" {
		log.Println("WARNING: master_authorization_key not set")
	}

	mw := handlers.NewAuthMiddleware(accountRepo, authKey)
	accountApiGroup := router.Group("/api/account")
	appGroup := router.Group("/api/app")
	dbGroup := router.Group("/api/db")

	appGroup.Use(mw.JwtVerifyHandler)
	accountApiGroup.Use(mw.MasterAuthorizationMiddleware)

	router.POST("/api/build", appHandler.BuildAppHandler)
	appGroup.GET("/logs", appHandler.LogsHandler)
	appGroup.POST("/new", appHandler.CreateAppHandler)
	accountApiGroup.POST("/new", accountHandler.CreateNewUserHandler)
	dbGroup.POST("/new", appHandler.ProvisionDbHandler)
	accountApiGroup.POST("/authenticate", accountHandler.AuthenticateAccount)

	// run tcp server
	go func() {
		if err := tcp.Run(); err != nil {
			log.Fatal("failed to start TCP server ", err)
		}
	}()

	// run git-http server
	http.Handle("/", service.Server)
	go func() {
		gitServerAddr := fmt.Sprintf(":%s", os.Getenv("GIT_SERVER_ADDR"))
		log.Println("git server accepting http request on", gitServerAddr)
		if err := http.ListenAndServe(gitServerAddr, nil); err != nil {
			log.Fatal("failed to start git server ", err)
		}
	}()
	log.Printf("appear server serving on %s", addr)
	if err := router.Run(addr); err != nil {
		return err
	}
	return nil
}

func (s *Server) createDbConnection(uri string) (*gorm.DB, error) {
	db, err := gorm.Open("postgres", uri)
	if err != nil {
		return nil, err
	}
	if err := db.DB().Ping(); err != nil {
		return nil, err
	}
	s.runMigration(db)
	return db, nil
}

func (s *Server) parseConfig() error {
	configDir := s.configDir
	if _, err := os.Stat(configDir); err != nil {
		configDir = filepath.Join(configDir, "appear_config.json")
		if _, err := os.Stat(configDir); err != nil {
			return err
		}
	}
	data, err := ioutil.ReadFile(configDir)
	if err != nil {
		return err
	}
	c := &config.Configuration{}
	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to parse json config: %v", err)
	}
	s.config = c
	return nil
}

func (s *Server) runMigration(db *gorm.DB) {
	db.AutoMigrate(&types.User{}, &types.App{}, &types.Credential{})
}
