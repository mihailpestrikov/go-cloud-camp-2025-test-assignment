package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-cloud-camp-2025-test-assignment/config"
	"go-cloud-camp-2025-test-assignment/internal/balancer"
	"go-cloud-camp-2025-test-assignment/internal/health"
	"go-cloud-camp-2025-test-assignment/internal/proxy"
	"go-cloud-camp-2025-test-assignment/internal/ratelimit"
	"go-cloud-camp-2025-test-assignment/internal/storage"
	"go-cloud-camp-2025-test-assignment/pkg/logger"
	"go-cloud-camp-2025-test-assignment/pkg/redis"

	"github.com/rs/zerolog/log"
)

func main() {

	configPath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	logger.Setup(cfg.Logging)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go handleSignals(cancel)

	loadBalancer, err := balancer.BalancerFactory(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create load balancer")
	}

	var rateLimiter ratelimit.RateLimiter
	var clientManager *ratelimit.ClientManager

	if cfg.RateLimit.Enabled {

		var store storage.Storage

		if cfg.RateLimit.Redis.Addr != "" {

			redisClient, err := redis.New(cfg.RateLimit.Redis)
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to create Redis client")
			}

			store, err = storage.NewRedisStorage(redisClient)
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to create Redis storage")
			}
		} else {

			store = storage.NewMemoryStorage()
		}

		defer store.Close()

		tbRateLimiter, err := ratelimit.NewTokenBucketRateLimiter(store, &cfg.RateLimit)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to create rate limiter")
		}
		defer tbRateLimiter.Close()

		rateLimiter = tbRateLimiter
		clientManager = ratelimit.NewClientManager(store, tbRateLimiter, &cfg.RateLimit)
	}

	healthChecker := health.NewHTTPHealthChecker(&cfg.HealthCheck)

	if cfg.HealthCheck.Enabled {
		go balancer.StartHealthChecks(ctx, loadBalancer, &cfg.HealthCheck, healthChecker)
	}

	proxyServer := proxy.NewProxy(
		loadBalancer,
		cfg,
		proxy.WithRateLimiter(rateLimiter),
	)

	mux := http.NewServeMux()

	mux.Handle("/", proxyServer)

	if clientManager != nil {
		clientManager.RegisterHandlers(mux)
		mux.HandleFunc("/client-status", clientManager.HandleStatus)
	}

	mux.HandleFunc("/lb-status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","balancer":"%s","backends":%d}`,
			loadBalancer.Name(), len(loadBalancer.GetHealthyBackends()))
	})

	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := loadBalancer.GetStatistics()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  cfg.Server.Timeout,
		WriteTimeout: cfg.Server.Timeout,
		IdleTimeout:  120 * time.Second,
	}

	log.Info().Int("port", cfg.Server.Port).Msg("Starting HTTP server")
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("Failed to start HTTP server")
		}
	}()

	<-ctx.Done()

	log.Info().Msg("Shutting down HTTP server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	log.Info().Msg("Server gracefully stopped")
}

func handleSignals(cancel context.CancelFunc) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	<-signalChan
	log.Info().Msg("Received termination signal")
	cancel()
}
