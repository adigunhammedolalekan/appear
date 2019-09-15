package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

type Configuration struct {
	DatabaseUrl     string    `json:"database_url"`
	GitStoragePath  string    `json:"git_storage_path"`
	DockerBuildPath string    `json:"docker_build_path"`
	Auth            *Auth     `json:"auth"`
	K8sConfigDir    string    `json:"k8s_config_dir"`
	Host            string    `json:"host"`
	Registry        *Registry `json:"registry"`
}

type Auth struct {
	MasterAuthKey string `json:"master_authorization_key"`
}

type Registry struct {
	RegistryUrl string `json:"url"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}

func InitDefaultConfig() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	configPath := filepath.Join(wd, "appear_config.json")
	defaultConfig := &Configuration {
		GitStoragePath:  "/mnt/repos",
		DockerBuildPath: "/mnt/build",
		Auth:            &Auth{MasterAuthKey: randomBase64String()},
		Registry:        &Registry{RegistryUrl: "localhost:5000/"},
		DatabaseUrl:     "postgres://postgres:man@localhost:5432/appear?sslmode=disable",
	}
	data, err := json.Marshal(defaultConfig)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(configPath, data, os.ModePerm)
}

func randomBase64String() string {
	hasher := sha256.New()
	hasher.Write([]byte(time.Now().String() + uuid.New().String()))
	s := hasher.Sum(nil)
	return base64.StdEncoding.EncodeToString(s)
}
