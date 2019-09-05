package handlers

import (
	"github.com/adigunhammedolalekan/paas/repos"
	"github.com/adigunhammedolalekan/paas/types"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

const tokenKey = "auth_token_key"

type AuthMiddleWare struct {
	repo          repos.UserRepository
	masterAuthKey string
}

func NewAuthMiddleware(repo repos.UserRepository, authKey string) *AuthMiddleWare {
	return &AuthMiddleWare{repo: repo, masterAuthKey: authKey}
}

func (mw *AuthMiddleWare) JwtVerifyHandler(ctx *gin.Context) {
	token := ctx.GetHeader("Authorization")
	if token == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, &Response{Error: true, Message: "authentication token is missing"})
		return
	}

	parts := strings.Split(token, " ")
	if len(parts) != 2 {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, &Response{Error: true, Message: "authentication token is missing"})
		return
	}

	user, err := mw.repo.VerifyToken(parts[1])
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, &Response{Error: true, Message: err.Error()})
		return
	}

	ctx.Set(tokenKey, user)
	ctx.Next()
}

func (mw *AuthMiddleWare) MasterAuthorizationMiddleware(ctx *gin.Context) {
	token := ctx.GetHeader("X-Master-Authorization")
	if token != mw.masterAuthKey {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, &Response{Error: true, Message: "authorized"})
		return
	}
	ctx.Next()
}

func ParseToken(ctx *gin.Context) *types.User {
	in, ok := ctx.Get(tokenKey)
	if !ok {
		return nil
	}

	user, ok := in.(*types.User)
	if !ok {
		return nil
	}
	return user
}
