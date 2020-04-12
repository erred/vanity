package main

import (
	"context"
	"flag"
	stdlog "log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

//go:generate go run generate.go template.gohtml
const (
	redirectURL = "https://seankhliao.com/"
)

func main() {
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, NoColor: true})
	slg := stdlog.New(log.Logger, "", 0)

	s := NewServer(os.Args)

	// prometheus
	promhandler := promhttp.InstrumentMetricHandler(
		prometheus.DefaultRegisterer,
		promhttp.HandlerFor(
			prometheus.DefaultGatherer,
			promhttp.HandlerOpts{ErrorLog: slg},
		),
	)

	// routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.healthcheck)
	mux.Handle("/metrics", promhandler)
	mux.Handle("/", s)

	// server
	srv := &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slg,
	}
	go func() {
		err := srv.ListenAndServe()
		log.Info().Err(err).Msg("serve exit")
	}()

	// shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	sig := <-sigs
	log.Info().Str("signal", sig.String()).Msg("shutting down")
	srv.Shutdown(context.Background())
}

type Server struct {
	// config
	addr string
	tmpl string
	t    *template.Template

	// metrics
	module *prometheus.CounterVec
}

func NewServer(args []string) *Server {
	s := &Server{
		module: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "com_seabkhliao_go_requests",
			Help: "go module",
		},
			[]string{"module"},
		),
	}

	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	fs.StringVar(&s.addr, "addr", ":80", "host:port to serve on")
	fs.StringVar(&s.tmpl, "tmpl", "builtin", "template to use, takes a singe {{.Repo}}, 'builtin' uses built in")
	err := fs.Parse(args)
	if err != nil {
		log.Fatal().Err(err).Msg("parse flags")
	}

	if s.tmpl == "builtin" {
		s.t = template.Must(template.New("t").Parse(tmplStr))
	} else {
		s.t = template.Must(template.ParseGlob(s.tmpl))
	}

	log.Info().Str("addr", s.addr).Str("tmpl", s.tmpl).Msg("configured")
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

	err := s.t.Execute(w, map[string]string{"Repo": repo})
	if err != nil {
		log.Error().Str("path", r.URL.Path).Err(err).Msg("execute")
	}

	// record
	s.module.WithLabelValues(repo).Inc()
}

func (s *Server) healthcheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
