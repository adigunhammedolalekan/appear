package main

import (
	"fmt"
	"github.com/adigunhammedolalekan/paas/build"
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
	"log"
	"net/http"
	"os"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("failed to load env variables")
	}

	sv := NewServer()
	if err := sv.Run(":9060"); err != nil {
		log.Fatal("HTTP server error ", err)
	}
}

type Server struct {
	Router *gin.Engine
}

func NewServer() *Server {
	s := &Server{}
	s.Router = gin.Default()
	return s
}

func (s *Server) Run(addr string) error {
	router := s.Router
	db, err := s.createDbConnection(os.Getenv("DB_URL"))
	if err != nil {
		return err
	}

	service, err := git.NewService(db)
	if err != nil {
		return err
	}
	dockerClient, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	k8sService, err := k8s.NewK8sService()
	if err != nil {
		log.Fatal("failed to create k8sService ", err)
	}

	dockerService := build.NewDockerService(dockerClient)
	tcp := server.NewTcpServer(os.Getenv("TCP_SERVER_ADDR"))
	appRepo := repos.NewAppRepository(db, service, k8sService)
	accountRepo := repos.NewUserRepository(db)
	appHandler := handlers.NewAppsHandler(tcp, appRepo, dockerService)
	accountHandler := handlers.NewUserHandler(accountRepo)
	mw := handlers.NewAuthMiddleware(accountRepo)

	apiGroup := router.Group("/api")
	appGroup := router.Group("/api/app")
	appGroup.Use(mw.JwtVerifyHandler)

	apiGroup.POST("/build", appHandler.BuildAppHandler)
	apiGroup.GET("/logs", appHandler.LogsHandler)
	appGroup.POST("/new", appHandler.CreateAppHandler)
	apiGroup.POST("/account/new", accountHandler.CreateNewUserHandler)
	apiGroup.POST("/account/authenticate", accountHandler.AuthenticateAccount)

	// run tcp server
	go func() {
		log.Println("started TCP server at :9010")
		if err := tcp.Run(); err != nil {
			log.Fatal("failed to start TCP server ", err)
		}
	}()

	// run git-http server
	http.Handle("/", service.Server)
	go func() {
		log.Println("git server accepting http request on :9798")
		gitServerAddr := fmt.Sprintf(":%s", os.Getenv("GIT_SERVER_ADDR"))
		if err := http.ListenAndServe(gitServerAddr, nil); err != nil {
			log.Fatal("failed to start git server ", err)
		}
	}()

	if err := router.Run(addr); err != nil {
		return err
	}
	return nil
}

func (s *Server) createDbConnection(uri string) (*gorm.DB, error) {
	log.Println("Connecting to", uri)
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

func (s *Server) runMigration(db *gorm.DB) {
	db.AutoMigrate(&types.User{}, &types.App{}, &types.Credential{})
}
