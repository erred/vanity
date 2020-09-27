package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"strings"
	"text/template"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/trace"
	"go.seankhliao.com/usvc"
)

//go:generate go run generate.go template.gohtml

const (
	name        = "go.seankhliao.com/vanity"
	redirectURL = "https://seankhliao.com/"
)

func main() {
	os.Exit(usvc.Exec(context.Background(), &Server{}, os.Args))
}

type Server struct {
	// config
	tmpl *template.Template

	log    zerolog.Logger
	tracer trace.Tracer

	// metrics
	module *prometheus.CounterVec
}

func (s *Server) Flags(fs *flag.FlagSet) {}

func (s *Server) Setup(ctx context.Context, u *usvc.USVC) error {
	s.log = u.Logger
	s.tracer = global.Tracer(name)

	s.tmpl = template.Must(template.New("page").Parse(tmplStr))
	s.module = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "vanity_module_requests",
	}, []string{"mod"})
	u.ServiceMux.Handle("/", s)
	return nil
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, span := s.tracer.Start(r.Context(), "serve")
	defer span.End()

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
