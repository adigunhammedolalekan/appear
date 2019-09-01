package config

type Configuration struct {
	DatabaseUrl     string `json:"database_url"`
	GitStoragePath  string `json:"git_storage_path"`
	DockerBuildPath string `json:"docker_build_path"`
	Auth            struct {
		MasterAuthKey string `json:"master_authorization_key"`
	} `json:"auth"`
	K8sConfigDir string `json:"k_8_s_config_dir"`
	Host         string `json:"host"`
}
