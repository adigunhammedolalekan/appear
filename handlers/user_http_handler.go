package handlers

import (
	"github.com/adigunhammedolalekan/paas/repos"
	"github.com/adigunhammedolalekan/paas/types"
	"github.com/gin-gonic/gin"
	"net/http"
)

type UserHandler struct {
	repo repos.UserRepository
}

func NewUserHandler(repo repos.UserRepository) *UserHandler {
	return &UserHandler{repo: repo}
}

func (handler *UserHandler) CreateNewUserHandler(ctx *gin.Context) {
	opt := &types.CreateAccountOpts{}
	if err := ctx.ShouldBindJSON(opt); err != nil {
		ctx.JSON(http.StatusBadRequest, &Response{Error: true, Message: "bad request: malformed json body"})
		return
	}

	newAccount, err := handler.repo.CreateUserAccount(opt)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &Response{Error: true, Message: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, &Response{Error: false, Message: "account created", Data: newAccount})
}

func (handler *UserHandler) AuthenticateAccount(ctx *gin.Context) {
	opt := &types.AuthenticateAccountOpts{}
	if err := ctx.ShouldBindJSON(opt); err != nil {
		ctx.JSON(http.StatusBadRequest, &Response{Error: true, Message: "bad request: malformed JSON body"})
		return
	}

	account, err := handler.repo.AuthenticateUser(opt)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, &Response{Error: true, Message: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, &Response{Error: false, Message: "authentication successful", Data: account})
}
