package gitviewer

import (
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"net/url"
	"os"
	"path/filepath"
)

type Repo struct {
	Url       string `yaml:"url"`
	Dir       string `yaml:"dir"`
	ParsedUrl *url.URL
}

func (r *Repo) LocalRootPath() string {
	// Where the repository should be stored
	return filepath.Join("repos", r.ParsedUrl.Host, r.ParsedUrl.Path)
}

func (r *Repo) LocalPath() string {
	// Root of where the repository should be accessible from
	return filepath.Join(r.LocalRootPath(), r.Dir)
}

func (r *Repo) Refresh(auth map[string]*Auth) error {
	// Get auth if required
	var basicAuth *http.BasicAuth
	if auth, ok := auth[r.ParsedUrl.Host]; ok {
		basicAuth = &http.BasicAuth{
			Username: auth.Username,
			Password: auth.Password,
		}
	}

	// Check if local repository exists
	repoLocalPath := r.LocalRootPath()
	if _, err := os.Stat(repoLocalPath); os.IsNotExist(err) {
		// If it doesn't exist, try to clone it
		_, err = git.PlainClone(repoLocalPath, false, &git.CloneOptions{
			URL:      r.Url,
			Depth:    1,
			Progress: os.Stdout,
			Auth:     basicAuth,
		})
		if err != nil {
			return err
		}
	} else {
		// If it does, try to pull changes
		gr, err := git.PlainOpen(repoLocalPath)
		if err != nil {
			return err
		}
		w, err := gr.Worktree()
		if err != nil {
			return err
		}
		err = w.Pull(&git.PullOptions{
			RemoteName: "origin",
			Progress:   os.Stdout,
			Auth:       basicAuth,
		})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return err
		}
	}

	return nil
}
