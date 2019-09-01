package docker

import "log"

type Build interface {
	// docker language
	Stack() string
	// dependency manager used. e.g npm for Nodejs, dep for Go etc..
	Deps() string

	BaseImage() string

	EnvVars() []EnvVar

	Port() int64

	ExecCommand() string

	Name() string

	BaseDir() string
}

type EnvVar struct {
	Key   string
	Value string
}

type Config struct {
	Language string
	Dep      string
	Exec     string
	Name     string
	Envs     []EnvVar
	Host     string
}

func CreateBuildFromConfig(cfg *Config) Build {
	log.Println(cfg)
	switch cfg.Language {
	case "Go":
		return GolangBuild{}
	}
	return nodeJsBuildFromConfig(cfg)
}

type GolangBuild struct {
	dep     string
	envs    []EnvVar
	exec    string
	name    string
	baseDir string
}

func (g GolangBuild) Stack() string {
	return "Go"
}

func (g GolangBuild) Deps() string {
	return g.dep
}

func (g GolangBuild) BaseImage() string {
	return "golang:alpine"
}

func (g GolangBuild) EnvVars() []EnvVar {
	vars := make([]EnvVar, 0)
	for k := range g.envs {
		e := g.envs[k]
		env := EnvVar{Key: e.Key, Value: e.Value}
		vars = append(vars, env)
	}
	return vars
}

func (g GolangBuild) Port() int64 {
	return 80
}

func (g GolangBuild) ExecCommand() string {
	return g.exec
}

func (g GolangBuild) Name() string {
	return g.name
}

func (g GolangBuild) BaseDir() string {
	return g.baseDir
}

type NodeJsBuild struct {
	dep     string
	envs    map[string]string
	exec    string
	name    string
	baseDir string
}

func nodeJsBuildFromConfig(cfg *Config) NodeJsBuild {
	log.Println(cfg)
	return NodeJsBuild{
		dep:  cfg.Dep,
		exec: cfg.Exec,
		name: cfg.Name,
	}
}

func (g NodeJsBuild) Stack() string {
	return "NodeJs"
}

func (g NodeJsBuild) Deps() string {
	return g.dep
}

func (g NodeJsBuild) BaseImage() string {
	return "node:10"
}

func (g NodeJsBuild) EnvVars() []EnvVar {
	vars := make([]EnvVar, 0)
	for k := range g.envs {
		env := EnvVar{Key: k, Value: g.envs[k]}
		vars = append(vars, env)
	}
	return vars
}

func (g NodeJsBuild) Port() int64 {
	return 80
}

func (g NodeJsBuild) ExecCommand() string {
	return g.exec
}

func (g NodeJsBuild) Name() string {
	return g.name
}

func (g NodeJsBuild) BaseDir() string {
	return g.baseDir
}
