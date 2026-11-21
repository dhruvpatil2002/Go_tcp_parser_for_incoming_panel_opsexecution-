package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/shehbaaz/db-utopia/internal/config"
	"github.com/shehbaaz/db-utopia/internal/db"
	"github.com/shehbaaz/db-utopia/internal/logger"
	"github.com/shehbaaz/db-utopia/internal/models"
	"github.com/shehbaaz/db-utopia/internal/redis"
	"github.com/shehbaaz/db-utopia/internal/server"
	"github.com/shehbaaz/db-utopia/internal/worker"
)

func loadEnv() {
	// Helper function to check and load .env
	tryLoad := func(path string) bool {
		envPath := filepath.Join(path, ".env")

		if _, err := os.Stat(envPath); err == nil {
			if err := godotenv.Load(envPath); err == nil {
				log.Printf("[INFO] Loaded .env from: %s", envPath)
				return true
			}
		}

		return false
	}

	// 🔹 Try executable-based paths (production)
	if exe, err := os.Executable(); err == nil {

		exeDir := filepath.Dir(exe)
		parentDir := filepath.Dir(exeDir)

		pathsToCheck := []string{
			exeDir,    // current dir
			parentDir, // one level up
			filepath.Join(parentDir, "DBUTOPIATCPENV"), // parent/DBUTOPIATCPENV/.env
		}

		for _, p := range pathsToCheck {
			log.Printf("[INFO] Checking ENV Path: %s", filepath.Join(p, ".env"))

			if tryLoad(p) {
				return
			}
		}
	}

	// 🔹 Fallback to source-based path (go run / dev)
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot resolve source path")
	}

	projectRoot := filepath.Dir(filepath.Dir(file))
	parentDir := filepath.Dir(projectRoot)

	pathsToCheck := []string{
		projectRoot,
		parentDir,
		filepath.Join(parentDir, "DBUTOPIATCPENV"),
	}

	for _, p := range pathsToCheck {

		log.Printf("[INFO] Checking ENV Path: %s", filepath.Join(p, ".env"))

		if tryLoad(p) {
			return
		}
	}
	panic("failed to locate .env file")
}

func main() {
	loadEnv()
	currnetPath, err := os.Getwd()
	if err != nil {
		log.Fatalf("[FATAL] Failed to get current working directory: %v", err)
	}
	log.Printf("Current working directory: %s", currnetPath)

	cfg := config.New()
	logger.Setup(cfg)
	dbPool := db.NewPool(cfg.DBHost, cfg.DBName, cfg.DBUser, cfg.DBPassword)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workers := StartWorkers(ctx, dbPool)

	redisClient := redis.NewRedisClient(
		cfg.RedisHost,
		cfg.RedisPort,
		cfg.RedisPassword,
		cfg.RedisDB,
	)
	defer func() {
		if err := redisClient.Close(); err != nil {
			logger.Errorf("Failed to close redis connection  | err=%v", err)
		}
	}()

	ctxRedis := context.Background()
	err = redisClient.DeleteKeysByPrefix(ctxRedis)
	if err != nil {
		logger.Errorf("Failed to clear Redis cache: %v", err)
	}

	logger.Infof("Redis cache cleared successfully")
	
	panelDataCache := redis.NewPanelDataCache(redisClient)

	panelSensorCache := redis.NewPanelSensorCache(
		1*time.Hour,
		redisClient,
		dbPool,
	)

	zoneAlertCache := redis.NewZoneAlertCache(redisClient)
	panelZoneDataCache := redis.NewPanelZoneDataCache(redisClient)
	panelZoneStateCache := redis.NewPanelZoneStateCache(redisClient)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	srv := server.NewServer(cfg, cfg.TcpPort, dbPool, zoneAlertCache, panelZoneDataCache, panelZoneStateCache,
		panelSensorCache, panelDataCache, workers.LogQueue, workers.UpdateIPQueue, workers.HeartbeatQueue)

	go srv.Start(ctx)

	// TODO: Start the server
	logger.Info("Server is running...")

	// Block until Ctrl+C is pressed
	<-sig
	logger.Info("Received shutdown signal.")
	cancel()

	// Give goroutines time to clean up
	time.Sleep(5 * time.Second)
	srv.Stop()

	// Close queue AFTER server stops pushing logs
	close(workers.LogQueue)
	close(workers.UpdateIPQueue)
	close(workers.HeartbeatQueue)

	// Wait for workers to finish
	workers.Wg.Wait()
	logger.Info("Server shutdown gracefully.")

}

type Workers struct {
	LogQueue       chan models.PanelLog
	UpdateIPQueue  chan models.PanelIPUpdate
	HeartbeatQueue chan models.PanelHeartBeat
	Wg             sync.WaitGroup
}

func StartWorkers(ctx context.Context, dbPool *pgxpool.Pool) *Workers {

	w := &Workers{
		LogQueue:       make(chan models.PanelLog, 5000),
		UpdateIPQueue:  make(chan models.PanelIPUpdate, 5000),
		HeartbeatQueue: make(chan models.PanelHeartBeat, 1000),
	}

	n := 2

	for i := 1; i <= n; i++ {

		w.Wg.Add(1)
		go func(id int) {
			defer w.Wg.Done()
			worker.StartLogWorker(ctx, i, dbPool, w.LogQueue)
		}(i)

		w.Wg.Add(1)
		go func(id int) {
			defer w.Wg.Done()
			worker.StartUpdateIPWorker(ctx, i, dbPool, w.UpdateIPQueue)
		}(i)

		w.Wg.Add(1)
		go func(id int) {
			defer w.Wg.Done()
			worker.StartHeartbeatWorker(ctx, i, dbPool, w.HeartbeatQueue)
		}(i)
	}

	return w
}
