package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"text/template"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/metric"
	"go.seankhliao.com/usvc"
)

//go:generate go run generate.go template.gohtml

const (
	redirectURL = "https://seankhliao.com/"
)

func main() {
	var s Server

	srvc := usvc.DefaultConf()
	s.log = srvc.Logger()

	s.module = metric.Must(global.Meter(os.Args[0])).NewInt64Counter(
		"module_hit",
		metric.WithDescription("hits per module"),
	)

	s.tmpl = template.Must(template.New("page").Parse(tmplStr))

	m := http.NewServeMux()
	m.Handle("/", s)

	err := srvc.RunHTTP(context.Background(), m)
	if err != nil {
		s.log.Fatal().Err(err).Msg("run server")
	}
}

type Server struct {
	// config
	tmpl *template.Template

	// metrics
	module metric.Int64Counter

	log zerolog.Logger
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
