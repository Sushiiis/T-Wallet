// cmd/wallet/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"github.com/Sushiiis/T-Wallet/internal/auth"
	"github.com/Sushiiis/T-Wallet/internal/config"
	"github.com/Sushiiis/T-Wallet/internal/repository/postgres"
	grpcserver "github.com/Sushiiis/T-Wallet/internal/transport/grpc"
	httpserver "github.com/Sushiiis/T-Wallet/internal/transport/http"
	"github.com/Sushiiis/T-Wallet/internal/usecase"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("сервис остановлен с ошибкой", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger.Info("конфиг загружен", "env", cfg.Env)

	pool, err := postgres.New(ctx, cfg.Postgres.DSN())
	if err != nil {
		return err
	}
	defer pool.Close()
	logger.Info("подключение к postgres установлено")

	// Слои: репозитории -> usecase -> транспорт.
	userRepo := postgres.NewUserRepo(pool)
	walletRepo := postgres.NewWalletRepo(pool)
	tokens := auth.NewManager(cfg.JWT.Secret, cfg.JWT.TTL)
	uc := usecase.NewWallet(userRepo, walletRepo, tokens)
	handler := grpcserver.NewWalletHandler(uc)

	grpcSrv := grpcserver.New(handler, tokens)
	httpSrv, err := httpserver.New(ctx, ":"+cfg.HTTP.Port, pool, "localhost:"+cfg.GRPC.Port)
	if err != nil {
		return fmt.Errorf("build http server: %w", err)
	}

	lis, err := net.Listen("tcp", ":"+cfg.GRPC.Port)
	if err != nil {
		return fmt.Errorf("listen tcp: %w", err)
	}

	srvErr := make(chan error, 2)

	go func() {
		logger.Info("gRPC сервер слушает", "port", cfg.GRPC.Port)
		if err := grpcSrv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			srvErr <- fmt.Errorf("grpc serve: %w", err)
		}
	}()

	go func() {
		logger.Info("HTTP сервер слушает", "port", cfg.HTTP.Port)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- fmt.Errorf("http serve: %w", err)
		}
	}()

	select {
	case err := <-srvErr:
		return err
	case <-ctx.Done():
		logger.Info("получен сигнал остановки")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Shutdown)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http graceful shutdown не удался", "error", err)
	}

	stopped := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
		logger.Info("gRPC сервер остановлен корректно")
	case <-shutdownCtx.Done():
		logger.Warn("таймаут graceful shutdown, принудительная остановка gRPC")
		grpcSrv.Stop()
	}

	logger.Info("остановка завершена")
	return nil
}