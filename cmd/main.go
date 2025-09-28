package main

import (
    "context"
    "net/http"
    "os"
    "os/signal"
    "strings"
    "syscall"
    "time"
    "log/slog"
    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
    httpapi "github.com/tinoosan/ledger/internal/httpapi/v1"
    "github.com/tinoosan/ledger/internal/storage/memory"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

    // Logger (slog to stdout). Level via LOG_LEVEL; format via LOG_FORMAT (json|text, default json)
    logger := buildLoggerFromEnv()
    slog.SetDefault(logger)

    // In-memory repository for now
    store := memory.New()

    // Quick dev seeder: one user + two USD accounts + system OpeningBalances
    user := ledger.User{ID: uuid.New()}
    store.SeedUser(user)
    // System account reserved for opening balances
    opening := ledger.Account{ID: uuid.New(), UserID: user.ID, Name: "Opening Balances", Currency: "USD", Type: ledger.AccountTypeEquity, Method: "OpeningBalances", Vendor: "System", System: true, Active: true}
    cash := ledger.Account{ID: uuid.New(), UserID: user.ID, Name: "Cash", Currency: "USD", Type: ledger.AccountTypeAsset, Method: "Cash", Vendor: "Wallet", Active: true}
    income := ledger.Account{ID: uuid.New(), UserID: user.ID, Name: "Income", Currency: "USD", Type: ledger.AccountTypeRevenue, Method: "Salary", Vendor: "Employer", Active: true}
    store.SeedAccount(opening)
    store.SeedAccount(cash)
    store.SeedAccount(income)
    logger.Info("DEV seed", "user_id", user.ID.String(), "opening_balances_account_id", opening.ID.String(), "cash_account_id", cash.ID.String(), "income_account_id", income.ID.String())

    srvMux := httpapi.New(store, store, store, store, store, store, store, logger).Handler()

	srv := &http.Server{
		Addr:              ":8080",
        Handler:           srvMux,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
    go func() {
        logger.Info("ledger service listening", "addr", srv.Addr)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            errCh <- err
        }
    }()

	select {
	case <-ctx.Done():
		ctxShutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
        if err := srv.Shutdown(ctxShutdown); err != nil {
            logger.Error("server shutdown error", "err", err)
        }
    case err := <-errCh:
        logger.Error("server error", "err", err)
    }
}
 
// parseLogLevel maps env values to slog.Leveler
func parseLogLevel(s string) slog.Leveler {
    switch s {
    case "DEBUG", "debug":
        return slog.LevelDebug
    case "WARN", "WARNING", "warn", "warning":
        return slog.LevelWarn
    case "ERROR", "ERR", "error", "err":
        return slog.LevelError
    default:
        return slog.LevelInfo
    }
}

func buildLoggerFromEnv() *slog.Logger {
    level := parseLogLevel(os.Getenv("LOG_LEVEL"))
    format := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT")))
    if format == "text" {
        return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
    }
    // default to JSON
    return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
