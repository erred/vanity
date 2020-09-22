package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/api/metric"
	"go.seankhliao.com/stream"
	"go.seankhliao.com/usvc"
	"google.golang.org/grpc"
)

//go:generate go run generate.go template.gohtml

const (
	name        = "vanity"
	redirectURL = "https://seankhliao.com/"
)

func main() {
	var s Server

	usvc.Run(context.Background(), name, &s, false)
}

type Server struct {
	// config
	tmpl       *template.Template
	streamAddr string
	client     stream.StreamClient
	cc         *grpc.ClientConn

	// metrics
	module metric.Int64Counter

	log zerolog.Logger
}

func (s *Server) Flag(fs *flag.FlagSet) {
	fs.StringVar(&s.streamAddr, "stream.addr", "stream:80", "url to connect to stream")
}

func (s *Server) Register(c *usvc.Components) error {
	s.log = c.Log
	s.tmpl = template.Must(template.New("page").Parse(tmplStr))

	s.module = metric.Must(c.Meter).NewInt64Counter(
		"module_request_total",
		metric.WithDescription("requests per module"),
	)
	c.HTTP.Handle("/", s)

	var err error
	s.cc, err = grpc.Dial(s.streamAddr, grpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("connect to stream: %w", err)
	}
	s.client = stream.NewStreamClient(s.cc)
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.cc.Close()
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
