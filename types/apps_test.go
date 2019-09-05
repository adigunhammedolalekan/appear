package types

import "testing"

func TestNewApp(t *testing.T) {
	a := NewApp("app", "http://localhost/user/app", 1)
	if a.Name != "app" {
		t.Fatal()
	}
	if a.RepoUrl != "http://localhost/user/app" {
		t.Fatal()
	}
	if a.UserId != 1 {
		t.Fatal()
	}
}

func TestNewCredential(t *testing.T) {
	c := NewCredential(1)
	if c.AppId != 1 {
		t.Fatal()
	}
	if c.Secret == "" || len(c.Secret) < 64 {
		t.Fatal()
	}
}

func TestApp_DeploymentName(t *testing.T) {
	a := NewApp("app", "", 1)
	if a.DeploymentName() != "app-deployment" {
		t.Fatal()
	}
}
