package gitviewer

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"
)

type Auth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Config struct {
	Auth  map[string]*Auth `yaml:"auth"`
	Repos map[string]*Repo `yaml:"repos"`
}

// Refresh the config from the config.yml file
func (c *Config) Refresh() error {
	// Load the config file and parse the YAML
	data, err := ioutil.ReadFile(filepath.Join("repos", "config.yml"))
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(data, c)
	if err != nil {
		return err
	}

	// Normalise repo URLs
	for _, repo := range c.Repos {
		if !strings.Contains(repo.Url, "/") {
			repo.Url = c.Auth["github.com"].Username + "/" + repo.Url
		}
		if strings.Count(repo.Url, "/") == 1 {
			repo.Url = "https://github.com/" + repo.Url
		}
		repo.ParsedUrl, err = url.Parse(repo.Url)
		if err != nil {
			return err
		}
	}

	return nil
}

// Refreshes repositories stored in the config
func (c *Config) RefreshRepos() error {
	// Refresh the config file first
	err := c.Refresh()
	if err != nil {
		return err
	}

	// Clone/pull each repository
	for _, repo := range c.Repos {
		err = repo.Refresh(c.Auth)
		if err != nil {
			return err
		}
	}

	return nil
}
