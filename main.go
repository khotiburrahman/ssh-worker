package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/youruser/gosshtunnel/clashapi"
	"github.com/youruser/gosshtunnel/config"
	"github.com/youruser/gosshtunnel/health"
	"github.com/youruser/gosshtunnel/internal/clashconfig"
	"github.com/youruser/gosshtunnel/internal/logger"
	"github.com/youruser/gosshtunnel/internal/rules"
	"github.com/youruser/gosshtunnel/network"
	"github.com/youruser/gosshtunnel/qload"
	"github.com/youruser/gosshtunnel/socks5"
	"github.com/youruser/gosshtunnel/sshclient"
	"github.com/youruser/gosshtunnel/transport"
)

const version = "1.0.0"

// Global: worker aktif (untuk selector YACD).
var activeWorker atomic.Value

func main() {
	cfgPath := flag.String("config", "config.json", "path ke config JSON")
	yamlPath := flag.String("clash", "", "path ke config YAML Clash (opsional)")
	flag.Parse()

	log := logger.New("info").For(logger.CompApp)
	log.Event("app.boot").Info("starting",
		"code", logger.CodeAppBoot, "config", *cfgPath, "version", version)

	// 1. Load JSON
	cfg, err := config.Load(*cfgPath, log)
	if err != nil {
		os.Exit(1)
	}
	cfg.ApplyDefaults()
	if errsList := cfg.ValidateAll(); len(errsList) > 0 {
		for _, e := range errsList {
			log.Errorf(logger.CodeCfgInvalid, "config invalid", e)
		}
		os.Exit(1)
	}
	log = logger.New(cfg.LogLevel).For(logger.CompApp)

	// 2. Load YAML (opsional)
	var clashCfg *clashconfig.ClashConfig
	if *yamlPath != "" {
		clashCfg, err = clashconfig.Load(*yamlPath, log)
		if err != nil {
			os.Exit(1)
		}
		if err := clashCfg.Validate(); err != nil {
			log.Errorf(logger.CodeCfgInvalid, "clash yaml invalid", err)
			os.Exit(1)
		}
	}

	// 3. Rule engine
	eng, _, err := rules.Build(rules.BuildOptions{
		GeoSitePath: cfg.Rules.GeoSitePath,
		Config:      clashCfg,
	}, log)
	if err != nil {
		log.Errorf(logger.CodeRulParse, "rule build failed", err)
		os.Exit(1)
	}
	log.Info("rule engine ready",
		"rules", eng.Count(),
		"blocklist", countRejects(eng),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sigCh
		log.Event("app.shutdown").Info("signal received",
			"code", logger.CodeAppShutdown, "sig", s.String())
		cancel()
	}()

	// Shared trackers untuk Clash API
	traffic := clashapi.NewTrafficTracker()
	conns := clashapi.NewConnectionTracker()
	logs := clashapi.NewLogBroadcaster()

	// Health config
	healthCfg := health.Config{
		DegradedRTT:  cfg.Health.DegradedRTT,
		UnhealthyRTT: cfg.Health.UnhealthyRTT,
		DegradedErr:  cfg.Health.DegradedErr,
		UnhealthyErr: cfg.Health.UnhealthyErr,
		DegradedHold: cfg.Health.DegradedHold,
		HealHold:     cfg.Health.HealHold,
	}

	// Jalankan worker
	addrs := cfg.ListenAddrs()
	log.Info("starting workers", "count", len(addrs), "addrs", addrs)

	var wg sync.WaitGroup
	var workerRefs []clashapi.WorkerRef
	var refsMu sync.Mutex

	for i, addr := range addrs {
		wg.Add(1)
		go func(idx int, listenAddr string) {
			defer wg.Done()
			ref, err := runWorker(ctx, cfg, listenAddr, idx, log,
				traffic, conns, healthCfg, eng)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Errorf(logger.CodeWrkStop, "worker exited with error", err,
					"worker", idx, "listen", listenAddr)
				return
			}
			refsMu.Lock()
			workerRefs = append(workerRefs, ref)
			refsMu.Unlock()
		}(i, addr)
	}

	// Beri waktu worker init
	time.Sleep(2 * time.Second)

	// Clash API server
	if cfg.ClashAPI.Enable {
		apiListen := cfg.ClashAPI.Listen
		apiSecret := cfg.ClashAPI.Secret
		cors := cfg.ClashAPI.CORSOrigins
		extUI := ""
		if clashCfg != nil {
			if clashCfg.ExternalController != "" {
				apiListen = clashCfg.ExternalController
			}
			if clashCfg.Secret != "" {
				apiSecret = clashCfg.Secret
			}
			extUI = clashCfg.ExternalUI
		}

		refsMu.Lock()
		refs := append([]clashapi.WorkerRef{}, workerRefs...)
		refsMu.Unlock()

		api := clashapi.NewServer(clashapi.Options{
			Listen:      apiListen,
			Secret:      apiSecret,
			CORSOrigins: cors,
			Version:     version,
			Workers:     refs,
			Traffic:     traffic,
			Connections: conns,
			Logs:        logs,
			Logger:      log,
			RuleEngine:  eng,
			ClashConfig: clashCfg,
			ExternalUI:  extUI,
			ActiveWorker: func() string {
				if v := activeWorker.Load(); v != nil {
					return v.(string)
				}
				return ""
			},
		})

		go func() {
			if err := api.Start(ctx); err != nil {
				log.Errorf(logger.CodeClashAPI, "api server exited", err)
			}
		}()
		log.Info("clash dashboard ready",
			"ui", "http://"+apiListen+"/ui/",
			"api", "http://"+apiListen,
		)
	}

	wg.Wait()
	log.Event("app.exit").Info("bye")
}

func countRejects(e *rules.Engine) int {
	n := 0
	for _, r := range e.Rules() {
		if r.Adapter() == rules.AdapterReject {
			n++
		}
	}
	return n
}

func runWorker(
	ctx context.Context,
	cfg *config.Config,
	listenAddr string,
	idx int,
	root *logger.Logger,
	traffic *clashapi.TrafficTracker,
	conns *clashapi.ConnectionTracker,
	healthCfg health.Config,
	eng *rules.Engine,
) (clashapi.WorkerRef, error) {

	wlog := root.For(logger.CompWorker).Worker(idx, listenAddr).Event("worker.start")
	wlog.Info("worker booting", "code", logger.CodeWrkStart)
	defer wlog.Event("worker.stop").Info("worker stopping", "code", logger.CodeWrkStop)

	// 1. Network
	dialer, err := network.NewDialer(cfg, wlog)
	if err != nil {
		wlog.Errorf(logger.CodeWrkStart, "network dialer init failed", err)
		return clashapi.WorkerRef{}, err
	}
	wlog.Debug("network dialer ready",
		"type", cfg.Network.Type, "target", cfg.DialTarget())

	// 2. Transport
	tr := transport.New(dialer, cfg, wlog)

	// 3. QLoad
	ql := qload.New(qload.Config{
		MaxRetries:      3,
		Concurrency:     10,
		Timeout:         15 * time.Second,
		FailureThreshold: 5,
		Cooldown:         15 * time.Second,
		AdaptiveTimeout:  true,
	}, wlog)

	dialFn := func(dctx context.Context) (net.Conn, error) {
		return ql.Dial(dctx, tr.Dial)
	}

	// 4. SSH Manager
	mgr := sshclient.NewManager(ctx, cfg.SSH, cfg.KnownHosts, dialFn, healthCfg, wlog)

	for attempt := 0; attempt < 5; attempt++ {
		if err := mgr.Start(); err == nil {
			break
		} else {
			wlog.Warn("ssh start failed",
				"code", logger.CodeSSHConnect,
				"attempt", attempt+1,
				"err", err,
			)
			select {
			case <-ctx.Done():
				return clashapi.WorkerRef{}, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}
	if mgr.State() != sshclient.StateUp {
		err := fmt.Errorf("gagal membangun SSH setelah beberapa percobaan")
		wlog.Errorf(logger.CodeSSHConnect, "ssh never came up", err)
		return clashapi.WorkerRef{}, err
	}

	// Set sebagai active worker pertama (untuk YACD selector)
	activeWorker.CompareAndSwap(nil, fmt.Sprintf("worker-%d", idx))

	// 5. SOCKS5
	srv, err := socks5.New(socks5.Options{
		Listen:      listenAddr,
		Log:         wlog,
		MaxConns:    cfg.SOCKS5.MaxConns,
		IdleTimeout: cfg.SOCKS5.IdleTimeout,
		RuleEngine:  eng,
	}, mgr.Dial)
	if err != nil {
		wlog.Errorf(logger.CodeSKSServe, "socks5 init failed", err)
		return clashapi.WorkerRef{}, err
	}
	defer srv.Close()

	wlog.Info("worker ready", "listen", listenAddr)

	if err := srv.Serve(ctx); err != nil {
		wlog.Errorf(logger.CodeSKSServe, "socks5 serve ended", err)
	}
	mgr.Close()

	return clashapi.WorkerRef{
		Name:    fmt.Sprintf("worker-%d", idx),
		Addr:    listenAddr,
		Healthy: mgr.Healthy,
		Latency: func() time.Duration {
			return mgr.Health().Snapshot().RTTEWMA
		},
		Snapshot: func() interface{} {
			return mgr.Health().Snapshot()
		},
	}, nil
}