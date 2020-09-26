package main

import (
	"context"
	"flag"
	"net/http"
	"strings"
	"text/template"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/api/metric"
	"go.seankhliao.com/usvc"
)

//go:generate go run generate.go template.gohtml

const (
	name        = "vanity"
	redirectURL = "https://seankhliao.com/"
)

func main() {
	usvc.Run(context.Background(), name, &Server{}, false)
}

type Server struct {
	// config
	tmpl *template.Template

	// metrics
	module metric.Int64Counter

	log zerolog.Logger
}

func (s *Server) Flag(fs *flag.FlagSet) {
}

func (s *Server) Register(c *usvc.Components) error {
	s.log = c.Log
	s.tmpl = template.Must(template.New("page").Parse(tmplStr))

	s.module = metric.Must(c.Meter).NewInt64Counter(
		"module_request_total",
		metric.WithDescription("requests per module"),
	)
	c.HTTP.Handle("/", s)
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
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
		s.log.Error().Err(err).Msg("execute template")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
