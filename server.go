package gitviewer

import (
	"encoding/json"
	"github.com/dimfeld/httptreemux"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Set of image extensions to embed
var imageExtensions = map[string]bool{
	"png":  true,
	"jpg":  true,
	"jpeg": true,
	"svg":  true,
	"gif":  true,
}

type Server struct {
	config *Config
	// Main HTML template used for rendering pages
	tmpl *template.Template
	// Map of language extension to prism language identifier
	languages map[string]string
}

type Breadcrumb struct {
	Name string
	Url  string
	Bold bool
	Dir  bool
}

type File struct {
	Name string
	Url  string
	Dir  bool
}

type fileSorter struct {
	files []*File
}

func (s *fileSorter) Len() int {
	return len(s.files)
}

func (s *fileSorter) Swap(i, j int) {
	s.files[i], s.files[j] = s.files[j], s.files[i]
}

func (s *fileSorter) Less(i, j int) bool {
	a, b := s.files[i], s.files[j]
	if a.Dir == b.Dir {
		return a.Name < b.Name
	} else {
		return a.Dir
	}
}

type TemplateContext struct {
	Path          string
	Breadcrumbs   []*Breadcrumb
	Files         []*File
	Content       string
	PrismLanguage string
	Image         string
}

func (s *Server) refreshRepos() {
	// Try refresh repos and log the output
	err := s.config.RefreshRepos()
	if err != nil {
		log.Printf("err refreshing repos: %v", err)
	} else {
		log.Print("refreshed repos")
	}
}

func (s *Server) indexHandler(w http.ResponseWriter, r *http.Request, params map[string]string) {
	// Use the path handler at the root of the directory
	params["path"] = "."
	s.pathHandler(w, r, params)
}

func (s *Server) pathHandler(w http.ResponseWriter, r *http.Request, params map[string]string) {
	// Check & load the repo from the config
	repoName, relativePath := params["repo"], path.Clean(params["path"])
	var repo *Repo
	if potentialRepo, ok := s.config.Repos[repoName]; ok {
		repo = potentialRepo
	} else {
		http.NotFound(w, r)
		return
	}

	// Try to load & identify the path from the repo
	var isDir bool
	filePath := filepath.Join(repo.LocalPath(), relativePath)
	if stat, err := os.Stat(filePath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else {
		isDir = stat.IsDir()
	}

	// Check if we can't preview this file, if we can't, just return it as the response
	var prismLanguage string
	var isImage bool
	if !isDir {
		ext := filepath.Ext(filePath)[1:]
		raw := r.URL.Query().Get("raw") == "true"
		if _, ok := imageExtensions[ext]; ok && !raw {
			isImage = true
		} else if language, ok := s.languages[ext]; !ok || raw {
			http.ServeFile(w, r, filePath)
			return
		} else {
			prismLanguage = language
		}
	}

	// Build breadcrumbs for template
	cumulativePath := "/" + repoName
	breadcrumbs := []*Breadcrumb{{
		Name: repoName,
		Url:  cumulativePath,
		Bold: true,
		Dir:  true,
	}}
	// Check if we're looking at the root
	if relativePath == "." {
		// Make the first breadcrumb inactive
		breadcrumbs[0].Url = ""
	} else {
		// Split the path into directories/files
		pathSegments := strings.Split(relativePath, "/")
		pathSegmentCount := len(pathSegments)
		for i := 0; i < pathSegmentCount; i++ {
			// Whether this is the last path segment
			isLast := i == pathSegmentCount-1
			// Add to the path for breadcrumb links
			cumulativePath += "/" + pathSegments[i]
			url := cumulativePath
			// Make the last breadcrumb inactive
			if isLast {
				url = ""
			}
			// Append the breadcrumb
			breadcrumbs = append(breadcrumbs, &Breadcrumb{
				Name: pathSegments[i],
				Url:  url,
				// Only the first and last breadcrumbs should be bold (and we've already added
				// the first one)
				Bold: isLast,
				// We know we have a valid file path so if this isn't the last breadcrumb then
				// this must be a directory, otherwise if this is the end, just check whether
				// we're looking at a directory
				Dir: !isLast || (isLast && isDir),
			})
		}
	}

	// Create universal template context
	ctx := &TemplateContext{
		Path:        path.Join(repoName, relativePath),
		Breadcrumbs: breadcrumbs,
	}

	// Create directory listing if required
	if isDir {
		// Read all files/directories in directory
		files, err := ioutil.ReadDir(filePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Build slice of file structs
		ctx.Files = make([]*File, 0)
		fileCount := len(files)
		for i := 0; i < fileCount; i++ {
			file := files[i]
			name, dir := file.Name(), file.IsDir()
			if dir && name == ".git" {
				continue
			}
			ctx.Files = append(ctx.Files, &File{
				Name: name,
				Url:  path.Join("/"+repoName, relativePath, name),
				Dir:  dir,
			})
		}
		// Sort with directories first then alphabetical order
		sort.Sort(&fileSorter{ctx.Files})
	} else if isImage {
		// Otherwise, if it's an image embed it
		ctx.Image = path.Join("/"+repoName, relativePath) + "?raw=true"
	} else {
		// Otherwise, just return & syntax highlight the content
		content, err := ioutil.ReadFile(filePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ctx.Content = string(content)
		ctx.PrismLanguage = prismLanguage
	}

	// Recompile template each time during development
	s.tmpl = template.Must(template.ParseFiles("template.html"))
	// Render & return the template
	_ = s.tmpl.Execute(w, ctx)
}

func (s *Server) Run() {
	// Initialise config object & load template
	s.config = &Config{}
	s.tmpl = template.Must(template.ParseFiles("template.html"))

	// Load list of preview languages
	var languages []struct {
		Name       string      `json:"name"`
		Extensions interface{} `json:"extensions"`
	}
	languageData, err := ioutil.ReadFile("languages.json")
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(languageData, &languages)
	if err != nil {
		panic(err)
	}
	// Unpack JSON
	s.languages = make(map[string]string)
	for _, language := range languages {
		if language.Extensions == nil {
			s.languages[language.Name] = language.Name
		} else if extension, ok := language.Extensions.(string); ok {
			s.languages[extension] = language.Name
		} else if extensions, ok := language.Extensions.([]interface{}); ok {
			for _, extension := range extensions {
				s.languages[extension.(string)] = language.Name
			}
		}
	}

	// Refresh repos every hour
	s.refreshRepos()
	go func() {
		for range time.Tick(time.Hour) {
			s.refreshRepos()
		}
	}()

	// Start HTTP server
	r := httptreemux.New()
	r.UsingContext().Handle(http.MethodGet, "/static/*path", http.StripPrefix("/static", http.FileServer(http.Dir("static"))).ServeHTTP)
	r.GET("/:repo", s.indexHandler)
	r.GET("/:repo/*path", s.pathHandler)
	log.Fatal(http.ListenAndServe(":8080", r))
}
