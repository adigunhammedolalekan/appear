package build

type Build interface {
	// build language
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
	Dep string
	Exec string
	Name string
	Envs map[string]string
}

func CreateBuildFromConfig(cfg *Config) Build {
	switch cfg.Language {
	case "Go":
		return GolangBuild{}
	}
	return nodeJsBuildFromConfig(cfg)
}

type GolangBuild struct {
	dep     string
	envs    map[string]string
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
		env := EnvVar{Key: k, Value: g.envs[k]}
		vars = append(vars, env)
	}
	return vars
}

func (g GolangBuild) Port() int64 {
	return 9888
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
	dep string
	envs map[string]string
	exec string
	name string
	baseDir string
}

func nodeJsBuildFromConfig(cfg *Config) NodeJsBuild {
	return NodeJsBuild{
		dep: cfg.Dep,
		envs: cfg.Envs,
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
	return 9881
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