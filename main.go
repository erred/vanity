package main

import (
	"context"
	"crypto/tls"
	"flag"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/template"
	"time"

	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/metric"
	"go.opentelemetry.io/otel/api/unit"
	"go.opentelemetry.io/otel/exporters/metric/prometheus"
	"go.opentelemetry.io/otel/label"

	"github.com/rs/zerolog"
)

//go:generate go run generate.go template.gohtml

const (
	redirectURL = "https://seankhliao.com/"
)

func main() {
	var s Server
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&s.addr, "addr", ":8080", "listen addr")
	fs.StringVar(&s.tlsCert, "tls-cert", "", "tls cert file")
	fs.StringVar(&s.tlsKey, "tls-key", "", "tls key file")
	fs.Parse(os.Args[1:])

	promExporter, _ := prometheus.InstallNewPipeline(prometheus.Config{
		DefaultHistogramBoundaries: []float64{1, 5, 10, 50, 100},
	})
	s.meter = global.Meter(os.Args[0])
	s.module = metric.Must(s.meter).NewInt64Counter(
		"module_hit",
		metric.WithDescription("hits per module"),
	)
	s.latency = metric.Must(s.meter).NewInt64ValueRecorder(
		"serve_latency",
		metric.WithDescription("http response latency"),
		metric.WithUnit(unit.Milliseconds),
	)

	s.log = zerolog.New(os.Stdout).With().Timestamp().Logger()

	s.tmpl = template.Must(template.New("page").Parse(tmplStr))

	m := http.NewServeMux()
	m.Handle("/", s)
	m.Handle("/metrics", promExporter)
	m.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	m.HandleFunc("/debug/pprof/", pprof.Index)
	m.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	m.HandleFunc("/debug/pprof/profile", pprof.Profile)
	m.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	m.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv := &http.Server{
		Addr:              s.addr,
		Handler:           m,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		TLSConfig: &tls.Config{
			MinVersion:               tls.VersionTLS13,
			PreferServerCipherSuites: true,
		},
	}

	if s.tlsKey != "" && s.tlsCert != "" {
		cert, err := tls.LoadX509KeyPair(s.tlsCert, s.tlsKey)
		if err != nil {
			s.log.Error().Err(err).Msg("laod tls keys")
			return
		}
		srv.TLSConfig.Certificates = []tls.Certificate{cert}
	}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		<-c
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		go func() {
			<-c
			cancel()
		}()
		err := srv.Shutdown(ctx)
		if err != nil {
			s.log.Error().Err(err).Msg("unclean shutdown")
		}
	}()

	err := srv.ListenAndServe()
	if err != nil {
		s.log.Error().Err(err).Msg("serve")
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
	t := time.Now()

	// get data
	repo := strings.Split(r.URL.Path, "/")[1]
	remote := r.Header.Get("x-forwarded-for")
	if remote == "" {
		remote = r.RemoteAddr
	}
	ua := r.Header.Get("user-agent")

	defer func() {
		s.latency.Record(r.Context(), time.Since(t).Milliseconds())
		s.module.Add(r.Context(), 1, label.String("module", repo))

		s.log.Debug().Str("path", r.URL.Path).Str("src", remote).Str("user-agent", ua).Msg("served")
	}()

	// filter paths
	if r.URL.Path == "/" {
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	err := s.tmpl.Execute(w, map[string]string{"Repo": repo})
	if err != nil {
		s.log.Error().Err(err).Msg("execute template")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
