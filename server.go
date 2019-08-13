package main

import (
	"github.com/adigunhammedolalekan/paas/build"
	"github.com/adigunhammedolalekan/paas/git"
	"github.com/adigunhammedolalekan/paas/handlers"
	"github.com/adigunhammedolalekan/paas/repos"
	"github.com/adigunhammedolalekan/paas/server"
	"github.com/adigunhammedolalekan/paas/types"
	"github.com/docker/docker/client"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"log"
	"os"
)

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
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.39"))
	if err != nil {
		return err
	}

	dockerService := build.NewDockerService(dockerClient)

	tcp := server.NewTcpServer(os.Getenv("TCP_SERVER_ADDR"))
	appRepo := repos.NewAppRepository(db, service)
	appHandler := handlers.NewAppsHandler(tcp, appRepo, dockerService)
	apiGroup := router.Group("/api")
	apiGroup.POST("/build", appHandler.BuildAppHandler)

	// run tcp server
	go func() {
		if err := tcp.Run(); err != nil {
			log.Fatal("failed to start TCP server ", err)
		}
	}()

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
	db.AutoMigrate(&types.User{}, &types.App{}, &types.Credential{})
	return db, nil
}
