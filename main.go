package main

import (
	"flag"
	"net/http"
	"os"
	"strings"
	"text/template"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.seankhliao.com/usvc"
)

//go:generate go run generate.go template.gohtml

const (
	redirectURL = "https://seankhliao.com/"
)

func main() {
	server := NewServer(os.Args)
	server.svc.Log.Error().Err(usvc.Run(usvc.SignalContext(), server.svc)).Msg("exited")
}

type Server struct {
	// config
	tmpl *template.Template

	// metrics
	module *prometheus.CounterVec

	// server
	svc *usvc.ServerSimple
}

func NewServer(args []string) *Server {
	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	s := &Server{
		tmpl: template.Must(template.New("").Parse(tmplStr)),
		module: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "vanity_module_requests",
			Help: "go module",
		},
			[]string{"module"},
		),
		svc: usvc.NewServerSimple(usvc.NewConfig(fs)),
	}

	fs.Parse(args[1:])
	s.svc.Mux.Handle("/metrics", promhttp.Handler())
	s.svc.Mux.Handle("/", s)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// filter paths
	if r.URL.Path == "/" {
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	// get data
	repo := strings.Split(r.URL.Path, "/")[1]
	remote := r.Header.Get("x-forwarded-for")
	if remote == "" {
		remote = r.RemoteAddr
	}
	ua := r.Header.Get("user-agent")

	err := s.tmpl.Execute(w, map[string]string{"Repo": repo})
	if err != nil {
		s.svc.Log.Error().Str("path", r.URL.Path).Str("src", remote).Str("user-agent", ua).Err(err).Msg("execute")
	} else {
		s.svc.Log.Debug().Str("path", r.URL.Path).Str("src", remote).Str("user-agent", ua).Msg("served")
	}

	// record
	s.module.WithLabelValues(repo).Inc()
}
