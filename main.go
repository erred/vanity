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
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	redirectURL = "https://seankhliao.com/"
)

func main() {
	s := NewServer(os.Args[1:])

	// prometheus
	promhandler := promhttp.InstrumentMetricHandler(
		prometheus.DefaultRegisterer,
		promhttp.HandlerFor(
			prometheus.DefaultGatherer,
			promhttp.HandlerOpts{ErrorLog: s.stdlog},
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
		ErrorLog:          s.stdlog,
	}
	go func() {
		s.log.Errorw("serve exit", "err", srv.ListenAndServe())
	}()

	// shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	s.log.Errorw("caught", "signal", <-sigs)
	srv.Shutdown(context.Background())
}

type Server struct {
	log    *zap.SugaredLogger
	stdlog *log.Logger

	// config
	addr string
	tmpl string
	t    *template.Template

	// metrics
	module *prometheus.CounterVec
}

func NewServer(args []string) *Server {
	loggerp, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	loggers, err := zap.NewStdLogAt(loggerp, zapcore.ErrorLevel)
	if err != nil {
		log.Fatal(err)
	}

	s := &Server{
		log:    loggerp.Sugar(),
		stdlog: loggers,
		module: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "com_seabkhliao_go_requests",
			Help: "go module",
		},
			[]string{"module"},
		),
	}

	fs := flag.NewFlagSet("com-seankhliao-go", flag.ExitOnError)
	fs.StringVar(&s.addr, "addr", ":8080", "host:port to serve on")
	fs.StringVar(&s.tmpl, "tmpl", "builtin", "template to use, takes a singe {{.Repo}}, 'builtin' uses built in")
	err = fs.Parse(args)
	if err != nil {
		s.log.Fatalw("parse args", "err", err)
	}

	if s.tmpl == "builtin" {
		s.t = template.Must(template.New("t").Parse(tmplStr))
	} else {
		s.t = template.Must(template.ParseGlob(s.tmpl))
	}
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

	err := s.t.Execute(w, Module{repo})
	if err != nil {
		s.log.Errorw("execute", "path", r.URL.Path, "err", err)
	}

	s.module.WithLabelValues(repo).Inc()
}

func (s *Server) healthcheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

type Module struct {
	Repo string
}

var tmplStr = `
<!doctype html>
<html lang="en">
<meta name="go-import"
        content="go.seankhliao.com/{{ .Repo }}
        git https://github.com/seankhliao/{{ .Repo }}" />
<meta name="go-source"
        content="go.seankhliao.com/{{ .Repo }}
        https://github.com/seankhliao/{{ .Repo }}
        https://github.com/seankhliao/{{ .Repo }}/tree/master{/dir}
        https://github.com/seankhliao/{{ .Repo }}/blob/master{/dir}/{file}#L{line}" />
<meta http-equiv="refresh"
        content="5;url=https://godoc.org/go.seankhliao.com/{{ .Repo }}" />
<title>go.seankhliao.com/{{ .Repo }}</a>
<p>source: <a
        href="https://github.com/seankhliao/{{ .Repo }}"
        ping="https://log.seankhliao.com/api?trigger=ping&src=go.seankhliao.com/{{ .Repo }}&dst=github.com/seankhliao/{{ .Repo }}">
        github</a></p>
<p>docs: <a
        href="https://godoc.org/go.seankhliao.com/{{ .Repo }}"
        ping="https://log.seankhliao.com/api?trigger=ping&src=go.seankhliao.com/{{ .Repo }}&dst=godoc.org/go.seankhliao.com/{{ .Repo }}">
        godoc</a></p>
<script>
let ts0 = new Date();
window.addEventListener("unload", () => {
  ts1 = new Date();
  navigator.sendBeacon("https://log.seankhliao.com/api?trigger=beacon&src=go.seankhliao.com/{{ .Repo }}&time=" + (ts1 - ts0)/1000 + "s");
});
</script>
</html>
`
