package health

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/flare-foundation/go-flare-common/pkg/logger"
)

// Probes are tiny GETs; the timeouts only have to defeat a stuck peer.
const (
	readHeaderTimeout = 2 * time.Second
	readTimeout       = 5 * time.Second
	writeTimeout      = 5 * time.Second
	idleTimeout       = 30 * time.Second
	maxHeaderBytes    = 4 << 10
)

// Handler routes GET /healthy, /startup and /ready to s. Other methods answer 405,
// other paths 404.
func Handler(s *Status) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthy", func(w http.ResponseWriter, _ *http.Request) {
		writeProbe(w, nil) // alive as long as this runs
	})
	mux.HandleFunc("GET /startup", func(w http.ResponseWriter, _ *http.Request) {
		writeProbe(w, s.Startup())
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		writeProbe(w, s.Ready())
	})

	return mux
}

// writeProbe answers 200 with an empty body, or 503 with err as a plain-text reason.
func writeProbe(w http.ResponseWriter, err error) {
	w.Header().Set("Cache-Control", "no-store")

	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Length", "0")
	w.WriteHeader(http.StatusOK)
}

// Server serves the probe endpoints on a listener bound by Listen.
type Server struct {
	srv *http.Server
	ln  net.Listener
}

// Listen binds addr and returns a Server ready to Start. Binding here rather than in
// Start turns an unusable address into a startup failure.
func Listen(addr string, s *Status) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("binding health server: %w", err)
	}

	srv := &http.Server{
		Handler:           Handler(s),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	return &Server{srv: srv, ln: ln}, nil
}

// Addr returns the bound address, with the port resolved when addr asked for :0.
func (s *Server) Addr() string {
	return s.ln.Addr().String()
}

// Start serves probes in a goroutine until Close. A serve failure is only logged: the
// probes then fail and the orchestrator notices.
func (s *Server) Start() {
	go func() {
		err := s.srv.Serve(s.ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Errorf("health server: %v", err)
		}
	}()
}

// Close stops accepting probes and waits for in-flight ones, bounded by ctx.
func (s *Server) Close(ctx context.Context) error {
	err := s.srv.Shutdown(ctx)

	// Shutdown closes only the listeners Serve registered; without a Start the bind would leak
	if lnErr := s.ln.Close(); lnErr != nil && !errors.Is(lnErr, net.ErrClosed) {
		return errors.Join(err, lnErr)
	}

	return err
}
