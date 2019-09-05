package types

type CreateAccountOpts struct {
	Name, Email, Password string
}

type AuthenticateAccountOpts struct {
	Email, Password string
}

type CreateAppOpts struct {
	UserId uint   `json:"user_id"`
	Name   string `json:"name"`
}

type HookInfo struct {
	RepoName string `json:"repo_name"`
	RepoPath string `json:"repo_path"`
	OldRev   string `json:"old_rev"`
	NewRev   string `json:"new_rev"`
	Ref      string `json:"ref"`
	RefType  string `json:"ref_type"`
	RefName  string `json:"ref_name"`
	Key      string `json:"key"`
}

type ProvisionDatabaseOpts struct {
	Name                                      string
	Type                                      string
	Space                                     int64
	DefaultPort                               int32
	BaseImage                                 string
	Envs                                      map[string]string
	PasswordKey, UsernameKey, DatabaseNameKey string
	DataMountPath                             string
}

type ProvisionDatabaseRequest struct {
	UserId       uint   `json:"user_id"`
	DatabaseType string `json:"database_type"`
	DatabaseName string `json:"database_name"`
}

type DatabaseProvisionResult struct {
	Credential *DatabaseCredential
}

type DatabaseCredential struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	DatabaseName string `json:"database_name"`
	DbType       string `json:"db_type"`
	DatabaseHost string `json:"database_host"`
}

func DatabaseCredentialAsUri(dbType string, credential *DatabaseCredential) string {
	return ""
}
