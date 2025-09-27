package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
    "github.com/tinoosan/ledger/internal/httpapi"
    "github.com/tinoosan/ledger/internal/storage/memory"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

    // In-memory repository for now
    store := memory.New()

    // Quick dev seeder: one user + two USD accounts
    user := ledger.User{ID: uuid.New()}
    store.SeedUser(user)
    cash := ledger.Account{ID: uuid.New(), UserID: user.ID, Name: "Cash", Currency: "USD", Type: ledger.AccountTypeAsset}
    income := ledger.Account{ID: uuid.New(), UserID: user.ID, Name: "Income", Currency: "USD", Type: ledger.AccountTypeRevenue}
    store.SeedAccount(cash)
    store.SeedAccount(income)
    log.Printf("DEV seed -> user_id=%s cash_account_id=%s income_account_id=%s", user.ID, cash.ID, income.ID)

    srvMux := httpapi.New(store, store).Mux()

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
		log.Printf("ledger service listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		ctxShutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctxShutdown); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
	case err := <-errCh:
		log.Fatalf("server error: %v", err)
	}
}
 
