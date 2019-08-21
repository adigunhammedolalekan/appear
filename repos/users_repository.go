package repos

import (
	"errors"
	"github.com/adigunhammedolalekan/paas/fn"
	"github.com/adigunhammedolalekan/paas/types"
	"github.com/dgrijalva/jwt-go"
	"github.com/jinzhu/gorm"
	"golang.org/x/crypto/bcrypt"
	"os"
)

type UserRepository interface {
	CreateUserAccount(opt *types.CreateAccountOpts) (*types.User, error)
	GetUser(email string) *types.User
	VerifyPassword(input, hash string) bool
	GenerateToken(user *types.User) string
	VerifyToken(token string) (*types.User, error)
	AuthenticateUser(opt *types.AuthenticateAccountOpts) (*types.User, error)
	GetUserByAttr(attr string, value interface{}) (*types.User, error)
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (repo *userRepository) CreateUserAccount(opt *types.CreateAccountOpts) (*types.User, error) {
	if err := fn.ValidateEmail(opt.Email); err != nil {
		return nil, err
	}

	if exists := repo.userWithEmailExists(opt.Email); exists {
		return nil, errors.New("email already in use by another user")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(opt.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := types.NewUser(opt.Name, opt.Email, string(hashedPassword))
	if err := repo.db.Create(user).Error; err != nil {
		return nil, err
	}

	user.Token = repo.GenerateToken(user)
	return user, nil
}

func (repo *userRepository) AuthenticateUser(opt *types.AuthenticateAccountOpts) (*types.User, error) {
	if err := fn.ValidateEmail(opt.Email); err != nil {
		return nil, err
	}

	user := repo.GetUser(opt.Email)
	if user == nil {
		return nil, errors.New("invalid authentication credentials")
	}

	if ok := repo.VerifyPassword(opt.Password, user.Password); !ok {
		return nil, errors.New("invalid authentication credentials")
	}

	user.Token = repo.GenerateToken(user)
	return user, nil
}

func (repo *userRepository) GetUser(email string) *types.User {
	u, err := repo.GetUserByAttr("email", email)
	if err != nil {
		return nil
	}
	return u
}

func (repo *userRepository) GetUserByAttr(attr string, value interface{}) (*types.User, error) {
	u := &types.User{}
	err := repo.db.Table("users").Where(attr+" = ?", value).First(u).Error
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (repo *userRepository) userWithEmailExists(email string) bool {
	u := repo.GetUser(email)
	if u == nil {
		return false
	}
	return true
}

func (repo *userRepository) VerifyPassword(input, hash string) bool {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(input)); err != nil {
		return false
	}
	return true
}

func (repo *userRepository) GenerateToken(user *types.User) string {
	tk := &types.Token{
		Id:    user.ID,
		Email: user.Email,
	}
	token := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), tk)
	tkString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return ""
	}
	return tkString
}

func (repo *userRepository) VerifyToken(inputToken string) (*types.User, error) {
	token := &types.Token{}
	tk, err := jwt.ParseWithClaims(inputToken, token, func(token *jwt.Token) (i interface{}, e error) {
		return []byte(os.Getenv("JWT_SECRET")), nil
	})
	if err != nil {
		return nil, err
	}

	if !tk.Valid {
		return nil, errors.New("invalid token supplied")
	}

	user := repo.GetUser(token.Email)
	if user == nil {
		return nil, errors.New("token user not found")
	}

	return user, nil
}
