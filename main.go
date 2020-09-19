package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"strings"
	"text/template"

	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/metric"

	"github.com/rs/zerolog"
)

//go:generate go run generate.go template.gohtml

const (
	redirectURL = "https://seankhliao.com/"
)

func main() {
	var srvconf HTTPServerConf
	var s Server

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	srvconf.RegisterFlags(fs)
	fs.Parse(os.Args[1:])

	s.log = zerolog.New(os.Stdout).With().Timestamp().Logger()

	s.module = metric.Must(global.Meter(os.Args[0])).NewInt64Counter(
		"module_hit",
		metric.WithDescription("hits per module"),
	)

	s.tmpl = template.Must(template.New("page").Parse(tmplStr))

	m := http.NewServeMux()
	m.Handle("/", s)

	_, run, err := srvconf.Server(m, s.log)
	if err != nil {
		s.log.Error().Err(err).Msg("prepare server")
		os.Exit(1)
	}

	err = run(context.Background())
	if err != nil {
		s.log.Error().Err(err).Msg("exit")
		os.Exit(1)
	}
}

type Server struct {
	// config
	tmpl *template.Template

	// metrics
	meter   metric.Meter
	module  metric.Int64Counter
	latency metric.Int64ValueRecorder

	log zerolog.Logger

	addr    string
	tlsCert string
	tlsKey  string
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
