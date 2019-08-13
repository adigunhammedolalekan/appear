package repos

import (
	"fmt"
	"github.com/adigunhammedolalekan/paas/git"
	"github.com/adigunhammedolalekan/paas/types"
	"github.com/goombaio/namegenerator"
	"github.com/jinzhu/gorm"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"log"
	"os"
	"time"
)

type AppsRepository interface {
	CreateApp(opt *types.CreateAppOpts) (*types.App, error)
	AppExists(name string, user uint) bool
	ListApps(userId uint) ([]*types.App, error)
	CloneRepository(path, httpUrl string) (*object.Commit, error)
	LogDeploymentEvent(user string, appId uint) error
	GetAppByRepositoryUrl(repoUrl string) (*types.App, error)
}

type appsRepository struct {
	db         *gorm.DB
	userRepo   UserRepository
	gitService *git.GitService
}

func NewAppRepository(
	db *gorm.DB,
	service *git.GitService) AppsRepository {
	return &appsRepository{
		db:         db,
		gitService: service,
		userRepo:   NewUserRepository(db),
	}
}

func (repo appsRepository) CreateApp(opt *types.CreateAppOpts) (*types.App, error) {
	user, err := repo.userRepo.GetUserByAttr("id", opt.UserId)
	if err != nil {
		return nil, err
	}

	// automatically generate name
	if opt.Name == "" {
		opt.Name = repo.randomAppName()
	}

	if exists := repo.AppExists(opt.Name, user.ID); exists {
		return nil, fmt.Errorf("app with name %s already exists for your account", opt.Name)
	}

	if err := repo.gitService.CreateNewRepository(user.UniqueName(), opt.Name); err != nil {
		return nil, err
	}

	tx := repo.db.Begin()
	if err := tx.Error; err != nil {
		return nil, err
	}

	repoUrl := fmt.Sprintf("http://%s/%s/%s.git", os.Getenv("REPO_SERVER_BASE_URL"), user.UniqueName(), opt.Name)
	app := types.NewApp(opt.Name, repoUrl, user.ID)
	if err := tx.Create(app).Error; err != nil {
		log.Println("[CreateApp]: failed to create app ", err)
		tx.Rollback()
		return nil, err
	}

	cred := types.NewCredential(app.ID)
	if err := tx.Create(cred).Error; err != nil {
		log.Println("[CreateApp]: failed to create credential ", err)
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return app, nil
}

func (repo *appsRepository) ListApps(id uint) ([]*types.App, error) {
	apps := make([]*types.App, 0)
	err := repo.db.Table("apps").Where("user_id = ?", id).Find(&apps).Error
	if err != nil {
		return nil, err
	}

	for _, app := range apps {
		app.User, _ = repo.userRepo.GetUserByAttr("id", app.UserId)
	}
	return apps, nil
}

func (repo *appsRepository) AppExists(name string, userId uint) bool {
	a := &types.App{}
	err := repo.db.Table("apps").Where("name = ? AND user_id = ?", name, userId).First(a).Error
	if err != nil {
		return false
	}
	return true
}

func (repo *appsRepository) CloneRepository(path, httpUrl string) (*object.Commit, error) {
	return repo.gitService.CloneRepository(path, httpUrl)
}

func (repo *appsRepository) LogDeploymentEvent(user string, appId uint) error {
	return nil
}

func (repo *appsRepository) GetAppByRepositoryUrl(repoUrl string) (*types.App, error) {
	a := &types.App{}
	err := repo.db.Table("apps").Where("repo_url = ?", repoUrl).First(a).Error
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (repo *appsRepository) randomAppName() string {
	seed := time.Now().UTC().UnixNano()
	return namegenerator.NewNameGenerator(seed).Generate()
}
