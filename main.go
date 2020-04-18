package main

import (
	"context"
	"flag"
	"log"
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
)

//go:generate go run generate.go template.gohtml

const (
	redirectURL = "https://seankhliao.com/"
)

var (
	port = func() string {
		port := os.Getenv("PORT")
		if port == "" {
			port = ":8080"
		} else if port[0] != ':' {
			port = ":" + port
		}
		return port
	}()
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT)
		<-sigs
		cancel()
	}()

	// server
	s := NewServer(os.Args)
	s.Run(ctx)
}

type Server struct {
	// config
	tmpl *template.Template

	// metrics
	module *prometheus.CounterVec

	// server
	log zerolog.Logger
	mux *http.ServeMux
	srv *http.Server
}

func NewServer(args []string) *Server {
	s := &Server{
		tmpl: template.Must(template.New("").Parse(tmplStr)),
		module: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "vanity_module_requests",
			Help: "go module",
		},
			[]string{"module"},
		),
		log: zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, NoColor: true, TimeFormat: time.RFC3339}).With().Timestamp().Logger(),
		mux: http.NewServeMux(),
		srv: &http.Server{
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      5 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}

	s.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	s.mux.Handle("/metrics", promhttp.Handler())
	s.mux.Handle("/", s)

	s.srv.Handler = s.mux
	s.srv.ErrorLog = log.New(s.log, "", 0)

	var tmpl string
	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	fs.StringVar(&s.srv.Addr, "addr", port, "host:port to serve on")
	fs.StringVar(&tmpl, "tmpl", "builtin", "template to use, takes a singe {{.Repo}}")
	fs.Parse(args[1:])

	if tmpl != "builtin" {
		s.tmpl = template.Must(template.ParseGlob(tmpl))
	}

	s.log.Info().Str("addr", s.srv.Addr).Str("tmpl", tmpl).Msg("configured")
	return s
}

func (s *Server) Run(ctx context.Context) {
	errc := make(chan error)
	go func() {
		errc <- s.srv.ListenAndServe()
	}()

	var err error
	select {
	case err = <-errc:
	case <-ctx.Done():
		err = s.srv.Shutdown(ctx)
	}
	s.log.Error().Err(err).Msg("server exit")
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

	err := s.tmpl.Execute(w, map[string]string{"Repo": repo})
	if err != nil {
		s.log.Error().Str("path", r.URL.Path).Str("src", remote).Err(err).Msg("execute")
	} else {
		s.log.Debug().Str("path", r.URL.Path).Str("src", remote).Msg("served")
	}

	// record
	s.module.WithLabelValues(repo).Inc()
}
