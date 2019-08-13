package git

import (
	"fmt"
	"github.com/adigunhammedolalekan/paas/types"
	"github.com/jinzhu/gorm"
	"github.com/sosedoff/gitkit"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

const gitStoragePath = "/var/repos"
const postReceiveHookPath = "/var/repos/hooks_executor"

type GitService struct {
	Server *gitkit.Server
}

func NewService(db *gorm.DB) (*GitService, error) {
	service := gitkit.New(gitkit.Config{
		Dir:  gitStoragePath,
		Auth: true,
	})

	service.AuthFunc = func(credential gitkit.Credential, request *gitkit.Request) (b bool, e error) {
		log.Println(request.RepoPath, request.RepoName)
		return verifyRepositoryUser(db, credential.Username, credential.Password)
	}

	if err := service.Setup(); err != nil {
		return nil, err
	}
	return &GitService{Server: service}, nil
}

func (s *GitService) CreateNewRepository(appName, repoName string) error {
	fullpath := filepath.Join(gitStoragePath, appName, fmt.Sprintf("%s%s", repoName, ".git"))
	if err := os.MkdirAll(fullpath, os.ModePerm); err != nil {
		return err
	}

	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = fullpath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	if err := s.initPostReceiveHook(fullpath); err != nil {
		return err
	}

	return nil
}

func (s *GitService) initPostReceiveHook(fullpath string) error {
	hooksPath := filepath.Join(fullpath, "hooks")
	hookFiles, err := ioutil.ReadDir(hooksPath)

	// clean hooks dir
	if err == nil {
		for _, fi := range hookFiles {
			if err := os.RemoveAll(filepath.Join(hooksPath, fi.Name())); err != nil {
				return err
			}
		}
	}

	postReceiveHookPath := filepath.Join(hooksPath, "post-receive")
	if err := ioutil.WriteFile(postReceiveHookPath, []byte(s.postReceiveHookContent()), 0755); err != nil {
		return err
	}
	if err := s.writeHookExecutor(hooksPath); err != nil {
		return err
	}

	log.Println("written hook file!")
	return nil
}

func (s *GitService) writeHookExecutor(hookPath string) error {
	fullpath := filepath.Join(hookPath, "main.go")
	executorContent, err := ioutil.ReadFile(postReceiveHookPath)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(fullpath, executorContent, 0755); err != nil {
		return err
	}
	log.Println("wrote hook executor function!")
	return nil
}

func (s *GitService) postReceiveHookContent() string {
	return fmt.Sprintf("#!/bin/sh\necho %q\ndir=`pwd`\necho $dir\ngo run hooks/main.go", "executing post receive!")
}

func (s *GitService) CloneRepository(clonePath, gitUrl string) (*object.Commit, error) {
	repo, err := git.PlainClone(clonePath, true, &git.CloneOptions{
		URL:  gitUrl,
		Auth: &http.BasicAuth{},
	})

	if err != nil {
		return nil, err
	}

	h, err := repo.Head()
	if err != nil {
		return nil, err
	}
	return repo.CommitObject(h.Hash())
}

func verifyRepositoryUser(db *gorm.DB, username, password string) (bool, error) {
	u := &types.User{}
	err := db.Table("users").Where("email = ?", username).First(u).Error
	if err != nil {
		return false, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		return false, err
	}
	return true, nil
}
