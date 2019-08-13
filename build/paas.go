package build

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/google/uuid"
	"io"
	"log"
	"os"
	"strings"
)

const goBuildCommand = "RUN CGO_ENABLED=0 GOOS=linux go build -o %s -a -installsuffix cgo -ldflags '-w'\n"

type DockerService struct {
	client            *client.Client
	dockerFileBuilder bytes.Buffer
}

type DockerBuildResult struct {
	Tag      string
	PullPath string
	Log      chan string
}

func NewDockerService(client *client.Client) *DockerService {
	return &DockerService{client: client}
}

func (p *DockerService) PullImage(name string) error {
	ctx := context.Background()
	reader, err := p.client.ImagePull(ctx, name, types.ImagePullOptions{})
	if err != nil {
		return err
	}

	for {
		buf := make([]byte, 512)
		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Println("error occurred while reading data ", err)
			continue
		}

		log.Printf("Read: n = %d, value = %v", n, string(buf[:]))
	}
	return nil
}

func (p *DockerService) PushImage(name string) error {
	tag, err := p.tagImage(name)
	if err != nil {
		return err
	}

	ctx := context.Background()
	reader, err := p.client.ImagePush(ctx, tag, types.ImagePushOptions{RegistryAuth: "$$password$$"})
	if err != nil {
		return err
	}

	for {
		buf := make([]byte, 512)
		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Println("failed to read push response ", err)
			continue
		}

		log.Printf("Read: n => %d, Value = %v", n, string(buf[:]))
	}
	return nil
}

func (p *DockerService) tagImage(name string) (string, error) {
	ctx := context.Background()
	tag := p.md5()[:5]
	source, target := name, fmt.Sprintf("%s/%s:%s", "docker-registry:5000", name, tag)
	log.Println("tagging ", target)
	return target, p.client.ImageTag(ctx, source, target)
}

func (p *DockerService) BuildLocalImage(path string, build Build) (*DockerBuildResult, error) {
	err := p.buildDockerfile(path, build)
	if err != nil {
		return nil, err
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	buildCtx, err := p.createBuildContext(path, wd)
	if err != nil {
		return nil, err
	}

	tag := fmt.Sprintf("%s%s%s", "docker-registry:5000/", build.Name(), p.md5()[:6])
	ctx := context.Background()
	reader, err := p.client.ImageBuild(ctx, buildCtx, types.ImageBuildOptions{
		Dockerfile: "Dockerfile", PullParent: true, Tags: []string{tag}, Remove: true, NoCache: true,
	})
	if err != nil {
		return nil, err
	}

	res := &DockerBuildResult{
		Log: make(chan string, 1),
	}

	go func() {
		for {
			buf := make([]byte, 512)
			_, err := reader.Body.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Println("error reading response ", err)
				continue
			}
			res.Log <- string(buf[:])
		}
	}()
	return &DockerBuildResult{Tag: tag, PullPath: tag}, nil
}

func (p *DockerService) buildDockerfile(path string, build Build) error {

	if err := p.write(fmt.Sprintf("FROM %s\n", build.BaseImage())); err != nil {
		return err
	}
	envs := build.EnvVars()
	for k := range envs {
		next := envs[k]
		if err := p.write(fmt.Sprintf("ENV %s=%s\n", next.Key, next.Value)); err != nil {
			return err
		}
	}

	baseDir := "/usr/src/app/code"
	if build.BaseDir() != "" {
		baseDir = fmt.Sprintf("/go/src/%s/app", build.BaseDir())
	}

	if err := p.write(fmt.Sprintf("WORKDIR %s\n", baseDir)); err != nil {
		return err
	}

	if err := p.write(fmt.Sprintf("COPY . %s\n", baseDir)); err != nil {
		return err
	}

	if err := p.write("RUN ls\n"); err != nil {
		return err
	}

	if err := p.write(fmt.Sprintf("RUN %s\n", build.Deps())); err != nil {
		return err
	}

	if strings.Contains(build.BaseImage(), "golang") {
		if err := p.write(fmt.Sprintf(goBuildCommand, build.Name())); err != nil {
			return err
		}
	}

	if err := p.write(fmt.Sprintf("EXPOSE %d\n", build.Port())); err != nil {
		return err
	}

	if err := p.write(build.ExecCommand()); err != nil {
		return err
	}

	if _, err := os.Create(path); err != nil {
		return err
	}

	fi, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return err
	}

	data := p.dockerFileBuilder.Bytes()
	if _, err := fi.Write(data); err != nil {
		return err
	}
	return nil
}

func (p *DockerService) createBuildContext(dockerFile, path string) (io.Reader, error) {
	return archive.Tar(path, archive.Uncompressed)
}

func (p *DockerService) contains(name string) bool {
	paths := []string{"node_modules"}
	for _, v := range paths {
		if v == name {
			return true
		}
	}
	return false
}

func (p *DockerService) write(s string) error {
	_, err := p.dockerFileBuilder.Write([]byte(s))
	if err != nil {
		return err
	}
	return nil
}

func (p *DockerService) md5() string {
	m5 := md5.New()
	m5.Write([]byte(uuid.New().String()))
	return fmt.Sprintf("%+x", string(m5.Sum(nil)))
}
