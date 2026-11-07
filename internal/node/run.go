package node

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"example.com/go-block-lattice/internal/consensus"
	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/p2p"
	"example.com/go-block-lattice/internal/protocol"
	"example.com/go-block-lattice/internal/rpc"
)

// Run opens durable state before accepting traffic and joins every owned worker
// before closing storage. Cancel the context for a graceful shutdown.
func Run(ctx context.Context, cfg Config) error {
	cfg.defaults()
	if err := cfg.Validate(); err != nil {
		return err
	}
	genesis, err := protocol.LoadGenesis(cfg.Genesis)
	if err != nil {
		return fmt.Errorf("load genesis: %w", err)
	}
	key, err := loadRepresentative(cfg.RepresentativeKey)
	if err != nil {
		return fmt.Errorf("load representative: %w", err)
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	store, err := ledger.OpenWithOptions(cfg.DataDir, genesis, ledger.Options{MaxHistoryBytes: cfg.HistoryLimitBytes})
	if err != nil {
		return fmt.Errorf("open ledger: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = store.Close()
		}
	}()
	engine, err := consensus.New(consensus.Config{Store: store, PrivateKey: key, RoundTimeout: cfg.roundTimeout(), MaxPending: cfg.MaxPending})
	if err != nil {
		return fmt.Errorf("create consensus: %w", err)
	}
	network, err := p2p.New(p2p.Config{Store: store, Engine: engine, Peers: cfg.Peers, MaxPeers: cfg.MaxPeers})
	if err != nil {
		return err
	}
	api, err := rpc.New(rpc.Config{Store: store, Engine: engine, Token: cfg.RPCToken, Syncing: network.Syncing, PeerStats: network.Stats, PeerDiagnostics: network.Diagnostics})
	if err != nil {
		return err
	}
	rpcListener, err := net.Listen("tcp", cfg.RPCListen)
	if err != nil {
		return fmt.Errorf("listen RPC: %w", err)
	}
	p2pListener, err := net.Listen("tcp", cfg.P2PListen)
	if err != nil {
		_ = rpcListener.Close()
		return fmt.Errorf("listen P2P: %w", err)
	}
	handlers := &requestGroup{}
	rpcServer := server(handlers.handler(api.Handler()))
	peerServer := server(handlers.handler(network.Handler()))
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	engine.Subscribe(network.Publish)
	errorsCh := make(chan error, 4)
	var workers sync.WaitGroup
	start := func(run func() error) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := run(); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled) {
				select {
				case errorsCh <- err:
				default:
				}
			}
		}()
	}
	start(func() error { return engine.Run(child) })
	start(func() error { return network.Run(child) })
	start(func() error { return rpcServer.Serve(rpcListener) })
	start(func() error { return peerServer.Serve(p2pListener) })
	slog.Info("node started", "network", genesis.ID(), "rpc", rpcListener.Addr(), "p2p", p2pListener.Addr(), "representative", cfg.RepresentativeKey != "")
	var runError error
	select {
	case <-ctx.Done():
	case runError = <-errorsCh:
		slog.Error("node runtime failed", "error", runError)
	}
	// Stop listeners first. HTTP handlers may finish accepted durable operations before
	// consensus/network workers are canceled and the ledger is released.
	handlers.stop()
	api.Stop()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	var shutdown sync.WaitGroup
	var shutdownMu sync.Mutex
	for _, srv := range []*http.Server{rpcServer, peerServer} {
		shutdown.Add(1)
		go func(srv *http.Server) {
			defer shutdown.Done()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				_ = srv.Close()
				shutdownMu.Lock()
				runError = errors.Join(runError, fmt.Errorf("HTTP shutdown: %w", err))
				shutdownMu.Unlock()
			}
		}(srv)
	}
	shutdown.Wait()
	shutdownCancel()
	cancel()
	workers.Wait()
	handlers.wait()
	err = store.Close()
	closed = true
	if err != nil {
		runError = errors.Join(runError, fmt.Errorf("close ledger: %w", err))
	}
	slog.Info("node stopped")
	return runError
}

func server(handler http.Handler) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
}

// requestGroup closes admission before waiting, avoiding new WaitGroup additions
// after shutdown begins. A forced connection close alone does not join handlers.
type requestGroup struct {
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

func (g *requestGroup) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		if g.closed {
			g.mu.Unlock()
			http.Error(w, "node is shutting down", http.StatusServiceUnavailable)
			return
		}
		g.wg.Add(1)
		g.mu.Unlock()
		defer g.wg.Done()
		next.ServeHTTP(w, r)
	})
}
func (g *requestGroup) stop() { g.mu.Lock(); g.closed = true; g.mu.Unlock() }
func (g *requestGroup) wait() { g.wg.Wait() }
