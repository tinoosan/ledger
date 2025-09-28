package main

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	httpapi "github.com/tinoosan/ledger/internal/httpapi/v1"
	"github.com/tinoosan/ledger/internal/ledger"
	"github.com/tinoosan/ledger/internal/storage/memory"
	pgstore "github.com/tinoosan/ledger/internal/storage/postgres"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Logger (slog to stdout). Level via LOG_LEVEL; format via LOG_FORMAT (json|text, default json)
	logger := buildLoggerFromEnv()
	slog.SetDefault(logger)

	var srvMux http.Handler
	var closeFn func()

	if dsn := strings.TrimSpace(os.Getenv("DATABASE_URL")); dsn != "" {
		// Use Postgres store when DATABASE_URL is provided
		pg, err := pgstore.Open(ctx, dsn)
		if err != nil {
			logger.Error("failed to connect to postgres", "err", err)
			os.Exit(1)
		}
		closeFn = func() { pg.Close() }
		// Optional dev seed for compose/local
		if dev := strings.ToLower(strings.TrimSpace(os.Getenv("DEV_SEED"))); dev == "1" || dev == "true" || dev == "yes" {
			user, accs, err := pg.SeedDev(ctx)
			if err != nil {
				logger.Error("dev seed failed", "err", err)
			} else {
				logDevSeed(logger, "postgres", user, accs)
				printDevSeedBanner(user, accs)
			}
		}
		srvMux = httpapi.New(pg, pg, pg, pg, pg, pg, pg, logger).Handler()
		logger.Info("storage backend: postgres")
	} else {
		// Default to in-memory store with a small dev seed
		store := memory.New()
		user := ledger.User{ID: uuid.New()}
		store.SeedUser(user)
		opening := ledger.Account{ID: uuid.New(), UserID: user.ID, Name: "Opening Balances", Currency: "GBP", Type: ledger.AccountTypeEquity, Group: "opening_balances", Vendor: "System", System: true, Active: true}
		cash := ledger.Account{ID: uuid.New(), UserID: user.ID, Name: "Cash", Currency: "GBP", Type: ledger.AccountTypeAsset, Group: "cash", Vendor: "Wallet", Active: true}
		income := ledger.Account{ID: uuid.New(), UserID: user.ID, Name: "Income", Currency: "GBP", Type: ledger.AccountTypeRevenue, Group: "salary", Vendor: "Employer", Active: true}
		store.SeedAccount(opening)
		store.SeedAccount(cash)
		store.SeedAccount(income)
		logDevSeed(logger, "memory", user, []ledger.Account{opening, cash, income})
		printDevSeedBanner(user, []ledger.Account{opening, cash, income})
		srvMux = httpapi.New(store, store, store, store, store, store, store, logger).Handler()
		logger.Info("storage backend: memory")
	}

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
	if closeFn != nil {
		closeFn()
	}
}

// logDevSeed emits structured logs with useful IDs
func logDevSeed(l *slog.Logger, backend string, user ledger.User, accs []ledger.Account) {
	ids := map[string]string{}
	for _, a := range accs {
		if a.System && strings.EqualFold(a.Group, "opening_balances") {
			ids["opening_balances_account_id"] = a.ID.String()
		}
		if a.Type == ledger.AccountTypeAsset && strings.EqualFold(a.Group, "cash") {
			ids["cash_account_id"] = a.ID.String()
		}
		if a.Type == ledger.AccountTypeRevenue && strings.EqualFold(a.Group, "salary") {
			ids["income_account_id"] = a.ID.String()
		}
	}
	l.Info("DEV seed ("+backend+")", "user_id", user.ID.String(), "ids", ids)
}

// printDevSeedBanner prints a simple banner to stdout for easy copy/paste of IDs
func printDevSeedBanner(user ledger.User, accs []ledger.Account) {
	var openingID, cashID, incomeID string
	for _, a := range accs {
		if a.System && strings.EqualFold(a.Group, "opening_balances") {
			openingID = a.ID.String()
		}
		if a.Type == ledger.AccountTypeAsset && strings.EqualFold(a.Group, "cash") {
			cashID = a.ID.String()
		}
		if a.Type == ledger.AccountTypeRevenue && strings.EqualFold(a.Group, "salary") {
			incomeID = a.ID.String()
		}
	}
	fmt.Println("==================== DEV SEED ====================")
	fmt.Printf("user_id: %s\n", user.ID.String())
	if openingID != "" {
		fmt.Printf("opening_balances_account_id: %s\n", openingID)
	}
	if cashID != "" {
		fmt.Printf("cash_account_id: %s\n", cashID)
	}
	if incomeID != "" {
		fmt.Printf("income_account_id: %s\n", incomeID)
	}
	fmt.Println("==================================================")
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
