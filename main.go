package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/metric"
	"go.seankhliao.com/stream"
	"go.seankhliao.com/usvc"
	"google.golang.org/grpc"
)

//go:generate go run generate.go template.gohtml

const (
	redirectURL = "https://seankhliao.com/"
)

func main() {
	var s Server

	srvc := usvc.DefaultConf(&s)
	s.log = srvc.Logger()

	s.module = metric.Must(global.Meter(os.Args[0])).NewInt64Counter(
		"module_hit",
		metric.WithDescription("hits per module"),
	)

	s.tmpl = template.Must(template.New("page").Parse(tmplStr))

	cc, err := grpc.Dial(s.streamAddr)
	if err != nil {
		s.log.Error().Err(err).Msg("connect to stream")
	}
	defer cc.Close()
	s.client = stream.NewStreamClient(cc)

	m := http.NewServeMux()
	m.Handle("/", s)

	err = srvc.RunHTTP(context.Background(), m)
	if err != nil {
		s.log.Fatal().Err(err).Msg("run server")
	}
}

type Server struct {
	// config
	tmpl       *template.Template
	streamAddr string
	client     stream.StreamClient

	// metrics
	module metric.Int64Counter

	log zerolog.Logger
}

func (s *Server) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&s.streamAddr, "stream.addr", "stream:80", "url to connect to stream")
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	remote := r.Header.Get("x-forwarded-for")
	if remote == "" {
		remote = r.RemoteAddr
	}

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

	httpRequest := &stream.HTTPRequest{
		Timestamp: time.Now().Format(time.RFC3339),
		Method:    r.Method,
		Domain:    r.Host,
		Path:      r.URL.Path,
		Remote:    remote,
		UserAgent: r.UserAgent(),
		Referrer:  r.Referer(),
	}

	_, err = s.client.LogHTTP(ctx, httpRequest)
	if err != nil {
		s.log.Error().Err(err).Msg("write to stream")
	}
}
