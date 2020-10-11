package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"strings"
	"text/template"

	"go.seankhliao.com/vanity/internal/serve"
	"k8s.io/klog/v2"
)

//go:generate go run generate.go template.gohtml

const (
	name        = "go.seankhliao.com/vanity"
	redirectURL = "https://seankhliao.com/"
)

func main() {
	os.Exit(serve.Run(&Server{}))
}

type Server struct {
	// config
	tmpl *template.Template
}

func (s *Server) InitFlags(fs *flag.FlagSet) {}

func (s *Server) Setup(ctx context.Context, c *serve.Components) error {
	s.tmpl = template.Must(template.New("page").Parse(tmplStr))
	c.Mux.Handle("/", s)
	return nil
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// filter paths
	if r.URL.Path == "/" {
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	repo := strings.Split(r.URL.Path, "/")[1]
	err := s.tmpl.Execute(w, map[string]string{"Repo": repo})
	if err != nil {
		klog.ErrorS(err, "exec", "path", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
