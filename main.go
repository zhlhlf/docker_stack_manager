package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"docker_stack_manager/internal/api"
	"docker_stack_manager/internal/config"
	"docker_stack_manager/internal/db"
	"docker_stack_manager/internal/detector"
	dockerx "docker_stack_manager/internal/docker"
	"docker_stack_manager/internal/notify"
	"docker_stack_manager/internal/scheduler"
)

// Version is injected by CI: -ldflags "-X main.Version=<8-char-commit>"
var Version = "dev"

func main() {
	cfg := config.Load()

	store, err := db.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	defer store.Close()

	dockerClient, err := dockerx.New()
	if err != nil {
		log.Fatalf("init docker client: %v (set DOCKER_HOST or mount /var/run/docker.sock)", err)
	}
	defer dockerClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := dockerClient.Ping(ctx); err != nil {
		log.Printf("warning: docker ping failed: %v", err)
		log.Printf("service APIs will error until Docker Engine is reachable")
	} else {
		if info, err := dockerClient.Info(ctx); err == nil {
			log.Printf("docker engine connected: %s", info)
		} else {
			log.Printf("docker engine connected")
		}
	}
	cancel()

	ding := notify.NewDingTalk(cfg.DingURL)
	if ding.Enabled() {
		log.Println("dingtalk webhook enabled (DING_URL)")
	} else {
		log.Println("dingtalk webhook not set via env; can configure in settings UI")
	}

	engine := detector.New(store, dockerClient, ding)
	sched := scheduler.New(store, engine)
	sched.Start()
	defer sched.Stop()

	server := api.New(store, engine, sched)
	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("Docker Stack Manager %s listening on http://0.0.0.0%s", Version, cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
	log.Println("shutdown complete")
}