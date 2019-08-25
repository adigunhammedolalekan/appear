package types

import (
	"fmt"
	"github.com/adigunhammedolalekan/paas/server"
	"github.com/dgrijalva/jwt-go"
	"github.com/jinzhu/gorm"
	"strings"
)

type User struct {
	gorm.Model
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Token    string `json:"token" gorm:"-"`
}

type App struct {
	gorm.Model
	UserId     uint        `json:"user_id"`
	Name       string      `json:"app_name"`
	RepoUrl    string      `json:"repo_url"`
	AppUrl string `json:"app_url"`
	ImageName string `json:"image_name"`
	Credential *Credential `json:"credential" gorm:"-"`
	User       *User       `json:"user" gorm:"-"`
}

type Token struct {
	jwt.StandardClaims
	Id    uint
	Email string
}

type Credential struct {
	gorm.Model
	Secret string `json:"secret"`
	AppId  uint   `json:"app_id"`
}

func NewUser(name, email, password string) *User {
	return &User{Name: name, Email: email, Password: password}
}

func NewApp(name, repoUrl string, userId uint) *App {
	return &App{Name: name, RepoUrl: repoUrl, UserId: userId}
}

func NewCredential(appId uint) *Credential {
	return &Credential{AppId: appId, Secret: server.GenerateRandomString(64)}
}

func (u *User) UniqueName() string {
	parts := strings.Split(u.Email, "@")
	if len(parts) != 2 {
		return u.Email
	}
	return parts[0]
}

func (a *App) DeploymentName() string {
	return fmt.Sprintf("%s-%s", a.Name, "deployment")
}