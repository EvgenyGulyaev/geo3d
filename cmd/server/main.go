package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/evgeny/3d-maps/internal/api"
	"github.com/evgeny/3d-maps/internal/cache"
	"github.com/evgeny/3d-maps/internal/config"
)

func main() {
	// Загрузка конфигурации из .env и переменных окружения
	cfg := config.Load()

	// Инициализация структурированного логирования через log/slog
	var logHandler slog.Handler
	if cfg.LogFormat == "json" {
		logHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		logHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	slog.Info("Starting 3D Maps Generator...",
		"env", cfg.Env,
		"port", cfg.Port,
		"cache_dir", cfg.CacheDir,
		"cache_size", cfg.CacheSize,
	)

	// Инициализация кэша
	modelCache := cache.New(cfg.CacheSize, cfg.CacheDir)

	// Создаём обработчик и роутер с настройками из конфига
	handler := api.NewHandler(modelCache, cfg)
	router := api.NewRouter(handler)

	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Канал для отслеживания ошибок запуска сервера
	serverErrors := make(chan error, 1)

	// Запуск HTTP-сервера в отдельной горутине
	go func() {
		slog.Info("Server is listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	// Канал для перехвата системных сигналов прерывания/завершения
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Ожидание сигнала или ошибки запуска сервера
	select {
	case err := <-serverErrors:
		slog.Error("Server start failed", "error", err)
		os.Exit(1)

	case sig := <-shutdown:
		slog.Info("Shutdown signal received", "signal", sig.String())

		// Контекст с таймаутом для завершения активных запросов
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		slog.Info("Shutting down server gracefully...")
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("Graceful shutdown failed, forcing close", "error", err)
			if err := srv.Close(); err != nil {
				slog.Error("Failed to close server", "error", err)
			}
			os.Exit(1)
		}

		slog.Info("Server stopped gracefully")
	}
}
