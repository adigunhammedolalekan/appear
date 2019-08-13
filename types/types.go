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
