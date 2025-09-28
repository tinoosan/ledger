package v1

import (
    "bytes"
    "encoding/json"
    "io"
    "log/slog"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/tinoosan/ledger/internal/ledger"
    "github.com/tinoosan/ledger/internal/storage/memory"
)

func testLogger() *slog.Logger {
    return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

type entryResp struct {
    ID            string         `json:"id"`
    UserID        string         `json:"user_id"`
    Date          time.Time      `json:"date"`
    Currency      string         `json:"currency"`
    Memo          string         `json:"memo"`
    Category      string         `json:"category"`
    Lines         []struct {
        ID          string `json:"id"`
        AccountID   string `json:"account_id"`
        Side        string `json:"side"`
        AmountMinor int64  `json:"amount_minor"`
    } `json:"lines"`
}

type acctResp struct {
    ID       string `json:"id"`
    UserID   string `json:"user_id"`
    Name     string `json:"name"`
    Currency string `json:"currency"`
    Type     string `json:"type"`
    Method   string `json:"method"`
    Vendor   string `json:"vendor"`
    Path     string `json:"path"`
}

type errResp struct {
    Error string `json:"error"`
    Code  string `json:"code"`
}

func setup(t *testing.T) (*memory.Store, http.Handler, uuid.UUID, ledger.Account, ledger.Account) {
    t.Helper()
    store := memory.New()
    user := ledger.User{ID: uuid.New()}
    store.SeedUser(user)
    cash := ledger.Account{ID: uuid.New(), UserID: user.ID, Name: "Cash", Currency: "USD", Type: ledger.AccountTypeAsset, Method: "Cash", Vendor: "Wallet"}
    income := ledger.Account{ID: uuid.New(), UserID: user.ID, Name: "Income", Currency: "USD", Type: ledger.AccountTypeRevenue, Method: "Salary", Vendor: "Employer"}
    store.SeedAccount(cash)
    store.SeedAccount(income)
    h := New(store, store, store, store, store, store, store, testLogger()).Handler()
    return store, h, user.ID, cash, income
}

func TestPostEntries_ValidAndInvalid(t *testing.T) {
    _, h, userID, cash, income := setup(t)

    // valid
    body := map[string]any{
        "user_id":        userID.String(),
        "date":           time.Now().UTC().Format(time.RFC3339),
        "currency":       "USD",
        "memo":           "Lunch",
        "category":       "eating_out",
        "lines": []map[string]any{
            {"account_id": cash.ID.String(), "side": "debit", "amount_minor": 1500},
            {"account_id": income.ID.String(), "side": "credit", "amount_minor": 1500},
        },
    }
    b, _ := json.Marshal(body)
    req := httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()
    h.ServeHTTP(rec, req)
    if rec.Code != http.StatusCreated {
        t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
    }
    var er entryResp
    if err := json.Unmarshal(rec.Body.Bytes(), &er); err != nil {
        t.Fatalf("decode: %v", err)
    }
    if er.Currency != "USD" || len(er.Lines) != 2 {
        t.Fatalf("unexpected response: %+v", er)
    }

    // invalid: unbalanced
    body["lines"] = []map[string]any{
        {"account_id": cash.ID.String(), "side": "debit", "amount_minor": 1500},
        {"account_id": income.ID.String(), "side": "credit", "amount_minor": 1400},
    }
    b, _ = json.Marshal(body)
    req = httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    rec = httptest.NewRecorder()
    reqDup := httptest.NewRequest(http.MethodPost, "/v1/accounts", bytes.NewReader(b))
    reqDup.Header.Set("Content-Type", "application/json")
    h.ServeHTTP(rec, reqDup)
    if rec.Code != http.StatusBadRequest {
        t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
    }
}

func TestEntries_ReverseAndList(t *testing.T) {
    _, h, userID, cash, income := setup(t)

    // create one entry
    body := map[string]any{
        "user_id":  userID.String(),
        "date":     time.Now().UTC().Format(time.RFC3339),
        "currency": "USD",
        "memo":     "Test",
        "category": "general",
        "lines": []map[string]any{
            {"account_id": cash.ID.String(), "side": "debit", "amount_minor": 500},
            {"account_id": income.ID.String(), "side": "credit", "amount_minor": 500},
        },
    }
    b, _ := json.Marshal(body)
    req := httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()
    h.ServeHTTP(rec, req)
    if rec.Code != http.StatusCreated { t.Fatalf("create entry expected 201, got %d", rec.Code) }
    var er entryResp
    _ = json.Unmarshal(rec.Body.Bytes(), &er)

    // reverse it
    rev := map[string]any{"user_id": userID.String(), "entry_id": er.ID}
    rb, _ := json.Marshal(rev)
    r2 := httptest.NewRequest(http.MethodPost, "/v1/entries/reverse", bytes.NewReader(rb))
    r2.Header.Set("Content-Type", "application/json")
    rec2 := httptest.NewRecorder()
    h.ServeHTTP(rec2, r2)
    if rec2.Code != http.StatusCreated { t.Fatalf("reverse expected 201, got %d: %s", rec2.Code, rec2.Body.String()) }
    var er2 entryResp
    _ = json.Unmarshal(rec2.Body.Bytes(), &er2)
    if len(er2.Lines) != 2 { t.Fatalf("expected 2 lines in reversal") }

    // list should have at least the two
    r3 := httptest.NewRequest(http.MethodGet, "/v1/entries?user_id="+userID.String(), nil)
    rec3 := httptest.NewRecorder()
    h.ServeHTTP(rec3, r3)
    if rec3.Code != http.StatusOK { t.Fatalf("list entries expected 200, got %d", rec3.Code) }
}

func TestEntries_GetAndIdempotency(t *testing.T) {
    _, h, userID, cash, income := setup(t)
    body := map[string]any{
        "user_id":  userID.String(),
        "date":     time.Now().UTC().Format(time.RFC3339),
        "currency": "USD",
        "memo":     "Test",
        "category": "general",
        "lines": []map[string]any{
            {"account_id": cash.ID.String(), "side": "debit", "amount_minor": 700},
            {"account_id": income.ID.String(), "side": "credit", "amount_minor": 700},
        },
    }
    b, _ := json.Marshal(body)
    req := httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder(); h.ServeHTTP(rec, req)
    if rec.Code != http.StatusCreated { t.Fatalf("create entry expected 201, got %d", rec.Code) }
    var er entryResp; _ = json.Unmarshal(rec.Body.Bytes(), &er)

    // GET /entries/{id}
    r := httptest.NewRequest(http.MethodGet, "/v1/entries/"+er.ID+"?user_id="+userID.String(), nil)
    rr := httptest.NewRecorder(); h.ServeHTTP(rr, r)
    if rr.Code != http.StatusOK { t.Fatalf("get entry expected 200, got %d", rr.Code) }

    // idempotency endpoint removed for now
}

func TestReclassify_BlockedAfterReverse(t *testing.T) {
    _, h, userID, cash, income := setup(t)
    // create one entry
    body := map[string]any{
        "user_id":  userID.String(),
        "date":     time.Now().UTC().Format(time.RFC3339),
        "currency": "USD",
        "memo":     "Test",
        "category": "general",
        "lines": []map[string]any{
            {"account_id": cash.ID.String(), "side": "debit", "amount_minor": 700},
            {"account_id": income.ID.String(), "side": "credit", "amount_minor": 700},
        },
    }
    b, _ := json.Marshal(body)
    req := httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder(); h.ServeHTTP(rec, req)
    if rec.Code != http.StatusCreated { t.Fatalf("create entry expected 201, got %d", rec.Code) }
    var er entryResp; _ = json.Unmarshal(rec.Body.Bytes(), &er)

    // reverse it
    rev := map[string]any{"user_id": userID.String(), "entry_id": er.ID}
    rb, _ := json.Marshal(rev)
    r1 := httptest.NewRequest(http.MethodPost, "/v1/entries/reverse", bytes.NewReader(rb))
    r1.Header.Set("Content-Type", "application/json")
    rr1 := httptest.NewRecorder(); h.ServeHTTP(rr1, r1)
    if rr1.Code != http.StatusCreated { t.Fatalf("reverse expected 201, got %d", rr1.Code) }

    // attempt reclassify -> 422 already_reversed
    corr := map[string]any{
        "user_id":  userID.String(),
        "entry_id": er.ID,
        "lines": []map[string]any{
            {"account_id": cash.ID.String(), "side": "debit", "amount_minor": 700},
            {"account_id": income.ID.String(), "side": "credit", "amount_minor": 700},
        },
    }
    cb, _ := json.Marshal(corr)
    r2 := httptest.NewRequest(http.MethodPost, "/v1/entries/reclassify", bytes.NewReader(cb))
    r2.Header.Set("Content-Type", "application/json")
    rr2 := httptest.NewRecorder(); h.ServeHTTP(rr2, r2)
    if rr2.Code != http.StatusUnprocessableEntity { t.Fatalf("expected 422, got %d: %s", rr2.Code, rr2.Body.String()) }
    var eresp errResp; _ = json.Unmarshal(rr2.Body.Bytes(), &eresp)
    if eresp.Code != "already_reversed" { t.Fatalf("expected already_reversed, got %q", eresp.Code) }
}

func TestReverse_AlreadyReversed(t *testing.T) {
    _, h, userID, cash, income := setup(t)
    // create one entry
    body := map[string]any{
        "user_id":  userID.String(),
        "date":     time.Now().UTC().Format(time.RFC3339),
        "currency": "USD",
        "memo":     "Test",
        "category": "general",
        "lines": []map[string]any{
            {"account_id": cash.ID.String(), "side": "debit", "amount_minor": 700},
            {"account_id": income.ID.String(), "side": "credit", "amount_minor": 700},
        },
    }
    b, _ := json.Marshal(body)
    req := httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder(); h.ServeHTTP(rec, req)
    if rec.Code != http.StatusCreated { t.Fatalf("create entry expected 201, got %d", rec.Code) }
    var er entryResp; _ = json.Unmarshal(rec.Body.Bytes(), &er)
    rev := map[string]any{"user_id": userID.String(), "entry_id": er.ID}
    rb, _ := json.Marshal(rev)
    // first reverse
    r1 := httptest.NewRequest(http.MethodPost, "/v1/entries/reverse", bytes.NewReader(rb))
    r1.Header.Set("Content-Type", "application/json")
    rr1 := httptest.NewRecorder(); h.ServeHTTP(rr1, r1)
    if rr1.Code != http.StatusCreated { t.Fatalf("reverse expected 201, got %d", rr1.Code) }
    // second reverse -> 422
    r2 := httptest.NewRequest(http.MethodPost, "/v1/entries/reverse", bytes.NewReader(rb))
    r2.Header.Set("Content-Type", "application/json")
    rr2 := httptest.NewRecorder(); h.ServeHTTP(rr2, r2)
    if rr2.Code != http.StatusUnprocessableEntity { t.Fatalf("expected 422, got %d: %s", rr2.Code, rr2.Body.String()) }
    var eresp errResp; _ = json.Unmarshal(rr2.Body.Bytes(), &eresp)
    if eresp.Code != "already_reversed" { t.Fatalf("expected already_reversed, got %q", eresp.Code) }
}

func TestEntries_IdempotencyHeader(t *testing.T) {
    _, h, userID, cash, income := setup(t)
    body := map[string]any{
        "user_id":  userID.String(),
        "date":     time.Now().UTC().Format(time.RFC3339),
        "currency": "USD",
        "memo":     "Test",
        "category": "general",
        "lines": []map[string]any{
            {"account_id": cash.ID.String(), "side": "debit", "amount_minor": 700},
            {"account_id": income.ID.String(), "side": "credit", "amount_minor": 700},
        },
    }
    b, _ := json.Marshal(body)
    // First request with Idempotency-Key
    r1 := httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
    r1.Header.Set("Content-Type", "application/json")
    r1.Header.Set("Idempotency-Key", "k-1")
    rr1 := httptest.NewRecorder(); h.ServeHTTP(rr1, r1)
    if rr1.Code != http.StatusCreated { t.Fatalf("expected 201, got %d: %s", rr1.Code, rr1.Body.String()) }
    var e1 entryResp; _ = json.Unmarshal(rr1.Body.Bytes(), &e1)

    // Second request with same Idempotency-Key should return 200 and same ID
    r2 := httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
    r2.Header.Set("Content-Type", "application/json")
    r2.Header.Set("Idempotency-Key", "k-1")
    rr2 := httptest.NewRecorder(); h.ServeHTTP(rr2, r2)
    if rr2.Code != http.StatusOK { t.Fatalf("expected 200, got %d: %s", rr2.Code, rr2.Body.String()) }
    var e2 entryResp; _ = json.Unmarshal(rr2.Body.Bytes(), &e2)
    if e1.ID != e2.ID { t.Fatalf("idempotency mismatch: %s vs %s", e1.ID, e2.ID) }

    // Without header should create a new entry (201) with a new ID
    r3 := httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
    r3.Header.Set("Content-Type", "application/json")
    rr3 := httptest.NewRecorder(); h.ServeHTTP(rr3, r3)
    if rr3.Code != http.StatusCreated { t.Fatalf("expected 201, got %d: %s", rr3.Code, rr3.Body.String()) }
    var e3 entryResp; _ = json.Unmarshal(rr3.Body.Bytes(), &e3)
    if e3.ID == e1.ID { t.Fatalf("expected new entry without idempotency header") }
}

func TestEntries_Validation422(t *testing.T) {
    _, h, userID, cash, income := setup(t)
    // too few lines
    body := map[string]any{
        "user_id":  userID.String(),
        "date":     time.Now().UTC().Format(time.RFC3339),
        "currency": "USD",
        "category": "general",
        "lines": []map[string]any{
            {"account_id": cash.ID.String(), "side": "debit", "amount_minor": 100},
        },
    }
    b, _ := json.Marshal(body)
    r := httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
    r.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder(); h.ServeHTTP(rr, r)
    if rr.Code != http.StatusUnprocessableEntity { t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String()) }
    var e errResp; _ = json.Unmarshal(rr.Body.Bytes(), &e)
    if e.Code != "too_few_lines" { t.Fatalf("expected code too_few_lines, got %q", e.Code) }

    // invalid amount
    body["lines"] = []map[string]any{
        {"account_id": cash.ID.String(), "side": "debit", "amount_minor": 0},
        {"account_id": income.ID.String(), "side": "credit", "amount_minor": 0},
    }
    b, _ = json.Marshal(body)
    r = httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
    r.Header.Set("Content-Type", "application/json")
    rr = httptest.NewRecorder(); h.ServeHTTP(rr, r)
    if rr.Code != http.StatusUnprocessableEntity { t.Fatalf("expected 422, got %d", rr.Code) }
    _ = json.Unmarshal(rr.Body.Bytes(), &e)
    if e.Code != "invalid_amount" { t.Fatalf("expected code invalid_amount, got %q", e.Code) }

    // mixed currency (entry EUR vs account USD)
    body["currency"] = "EUR"
    body["lines"] = []map[string]any{
        {"account_id": cash.ID.String(), "side": "debit", "amount_minor": 100},
        {"account_id": income.ID.String(), "side": "credit", "amount_minor": 100},
    }
    b, _ = json.Marshal(body)
    r = httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
    r.Header.Set("Content-Type", "application/json")
    rr = httptest.NewRecorder(); h.ServeHTTP(rr, r)
    if rr.Code != http.StatusUnprocessableEntity { t.Fatalf("expected 422, got %d", rr.Code) }
    _ = json.Unmarshal(rr.Body.Bytes(), &e)
    if e.Code != "mixed_currency" { t.Fatalf("expected code mixed_currency, got %q", e.Code) }

    // unbalanced
    body["currency"] = "USD"
    body["lines"] = []map[string]any{
        {"account_id": cash.ID.String(), "side": "debit", "amount_minor": 100},
        {"account_id": income.ID.String(), "side": "credit", "amount_minor": 90},
    }
    b, _ = json.Marshal(body)
    r = httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
    r.Header.Set("Content-Type", "application/json")
    rr = httptest.NewRecorder(); h.ServeHTTP(rr, r)
    if rr.Code != http.StatusUnprocessableEntity { t.Fatalf("expected 422, got %d", rr.Code) }
    _ = json.Unmarshal(rr.Body.Bytes(), &e)
    if e.Code != "unbalanced_entry" { t.Fatalf("expected code unbalanced_entry, got %q", e.Code) }
}

func TestEntries_Pagination(t *testing.T) {
    _, h, userID, cash, income := setup(t)
    // make 3 entries at increasing times
    base := time.Now().UTC().Add(-time.Minute)
    mk := func(ts time.Time, amt int64) {
        body := map[string]any{
            "user_id":  userID.String(),
            "date":     ts.Format(time.RFC3339),
            "currency": "USD",
            "category": "general",
            "lines": []map[string]any{
                {"account_id": cash.ID.String(), "side": "debit", "amount_minor": amt},
                {"account_id": income.ID.String(), "side": "credit", "amount_minor": amt},
            },
        }
        b, _ := json.Marshal(body)
        r := httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
        r.Header.Set("Content-Type", "application/json")
        rr := httptest.NewRecorder(); h.ServeHTTP(rr, r)
        if rr.Code != http.StatusCreated { t.Fatalf("create failed: %d %s", rr.Code, rr.Body.String()) }
    }
    mk(base.Add(0*time.Second), 100)
    mk(base.Add(1*time.Second), 200)
    mk(base.Add(2*time.Second), 300)

    // page 1
    r1 := httptest.NewRequest(http.MethodGet, "/v1/entries?user_id="+userID.String()+"&limit=2", nil)
    rr1 := httptest.NewRecorder(); h.ServeHTTP(rr1, r1)
    if rr1.Code != http.StatusOK { t.Fatalf("list page1 expected 200, got %d", rr1.Code) }
    var p1 struct{ Items []entryResp `json:"items"`; Next *string `json:"next_cursor"` }
    _ = json.Unmarshal(rr1.Body.Bytes(), &p1)
    if len(p1.Items) != 2 || p1.Next == nil { t.Fatalf("expected 2 items and cursor; got %+v", p1) }
    firstIDs := map[string]struct{}{p1.Items[0].ID: {}, p1.Items[1].ID: {}}

    // page 2
    r2 := httptest.NewRequest(http.MethodGet, "/v1/entries?user_id="+userID.String()+"&cursor="+*p1.Next, nil)
    rr2 := httptest.NewRecorder(); h.ServeHTTP(rr2, r2)
    if rr2.Code != http.StatusOK { t.Fatalf("list page2 expected 200, got %d", rr2.Code) }
    var p2 struct{ Items []entryResp `json:"items"`; Next *string `json:"next_cursor"` }
    _ = json.Unmarshal(rr2.Body.Bytes(), &p2)
    if len(p2.Items) != 1 || p2.Next != nil { t.Fatalf("expected 1 item and no cursor; got %+v", p2) }
    if _, ok := firstIDs[p2.Items[0].ID]; ok { t.Fatalf("duplicate id across pages") }
}

func TestNotFound_Standardized(t *testing.T) {
    _, h, userID, _, _ := setup(t)
    // entries/{id}
    rid := uuid.New().String()
    r := httptest.NewRequest(http.MethodGet, "/v1/entries/"+rid+"?user_id="+userID.String(), nil)
    rr := httptest.NewRecorder(); h.ServeHTTP(rr, r)
    if rr.Code != http.StatusNotFound { t.Fatalf("expected 404, got %d", rr.Code) }
    var e errResp; _ = json.Unmarshal(rr.Body.Bytes(), &e)
    if e.Error != "not_found" || e.Code != "not_found" { t.Fatalf("unexpected 404 body: %+v", e) }
    // accounts/{id}
    r = httptest.NewRequest(http.MethodGet, "/v1/accounts/"+rid+"?user_id="+userID.String(), nil)
    rr = httptest.NewRecorder(); h.ServeHTTP(rr, r)
    if rr.Code != http.StatusNotFound { t.Fatalf("expected 404 acc, got %d", rr.Code) }
    _ = json.Unmarshal(rr.Body.Bytes(), &e)
    if e.Error != "not_found" || e.Code != "not_found" { t.Fatalf("unexpected 404 body: %+v", e) }
    // balance
    r = httptest.NewRequest(http.MethodGet, "/v1/accounts/"+rid+"/balance?user_id="+userID.String(), nil)
    rr = httptest.NewRecorder(); h.ServeHTTP(rr, r)
    if rr.Code != http.StatusNotFound { t.Fatalf("expected 404 bal, got %d", rr.Code) }
    // ledger
    r = httptest.NewRequest(http.MethodGet, "/v1/accounts/"+rid+"/ledger?user_id="+userID.String(), nil)
    rr = httptest.NewRecorder(); h.ServeHTTP(rr, r)
    if rr.Code != http.StatusNotFound { t.Fatalf("expected 404 led, got %d", rr.Code) }
}

func TestLedger_BalanceConsistency(t *testing.T) {
    _, h, userID, cash, income := setup(t)
    // create 3 entries
    mk := func(amt int64) {
        body := map[string]any{
            "user_id":  userID.String(),
            "date":     time.Now().UTC().Format(time.RFC3339),
            "currency": "USD",
            "category": "general",
            "lines": []map[string]any{
                {"account_id": cash.ID.String(), "side": "debit", "amount_minor": amt},
                {"account_id": income.ID.String(), "side": "credit", "amount_minor": amt},
            },
        }
        b, _ := json.Marshal(body)
        r := httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
        r.Header.Set("Content-Type", "application/json")
        rr := httptest.NewRecorder(); h.ServeHTTP(rr, r)
        if rr.Code != http.StatusCreated { t.Fatalf("create failed: %d", rr.Code) }
    }
    mk(100); mk(200); mk(300)
    // balance
    rb := httptest.NewRequest(http.MethodGet, "/v1/accounts/"+cash.ID.String()+"/balance?user_id="+userID.String(), nil)
    rr := httptest.NewRecorder(); h.ServeHTTP(rr, rb)
    if rr.Code != http.StatusOK { t.Fatalf("balance expected 200, got %d", rr.Code) }
    var br struct{ BalanceMinor int64 `json:"balance_minor"` }
    _ = json.Unmarshal(rr.Body.Bytes(), &br)
    // ledger with limit large enough
    rl := httptest.NewRequest(http.MethodGet, "/v1/accounts/"+cash.ID.String()+"/ledger?user_id="+userID.String()+"&limit=100", nil)
    rlr := httptest.NewRecorder(); h.ServeHTTP(rlr, rl)
    if rlr.Code != http.StatusOK { t.Fatalf("ledger expected 200, got %d", rlr.Code) }
    var l struct{ Items []struct{ Running int64 `json:"running_balance_minor"` } `json:"items"` }
    _ = json.Unmarshal(rlr.Body.Bytes(), &l)
    if len(l.Items) == 0 { t.Fatalf("expected ledger items") }
    if l.Items[len(l.Items)-1].Running != br.BalanceMinor { t.Fatalf("running end %d != balance %d", l.Items[len(l.Items)-1].Running, br.BalanceMinor) }
}

func TestConcurrency_Smoke(t *testing.T) {
    _, h, userID, cash, income := setup(t)
    // post in parallel
    mk := func(amt int64) {
        body := map[string]any{
            "user_id":  userID.String(),
            "date":     time.Now().UTC().Format(time.RFC3339),
            "currency": "USD",
            "category": "general",
            "lines": []map[string]any{
                {"account_id": cash.ID.String(), "side": "debit", "amount_minor": amt},
                {"account_id": income.ID.String(), "side": "credit", "amount_minor": amt},
            },
        }
        b, _ := json.Marshal(body)
        r := httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
        r.Header.Set("Content-Type", "application/json")
        rr := httptest.NewRecorder(); h.ServeHTTP(rr, r)
        if rr.Code != http.StatusCreated { t.Fatalf("create failed: %d", rr.Code) }
    }
    const N = 5
    const M = 5
    done := make(chan struct{}, N*M)
    for i := 0; i < N; i++ {
        go func(i int) {
            for j := 0; j < M; j++ { mk(int64((i+1)*(j+1))) }
            done <- struct{}{}
        }(i)
    }
    for i := 0; i < N; i++ { <-done }
    // read all via pagination
    total := 0
    next := ""
    for {
        url := "/v1/entries?user_id="+userID.String()+"&limit=50"
        if next != "" { url += "&cursor=" + next }
        r := httptest.NewRequest(http.MethodGet, url, nil)
        rr := httptest.NewRecorder(); h.ServeHTTP(rr, r)
        if rr.Code != http.StatusOK { t.Fatalf("list expected 200, got %d", rr.Code) }
        var page struct{ Items []entryResp `json:"items"`; Next *string `json:"next_cursor"` }
        _ = json.Unmarshal(rr.Body.Bytes(), &page)
        total += len(page.Items)
        if page.Next == nil { break }
        next = *page.Next
    }
    if total < N*M { t.Fatalf("expected at least %d entries, got %d", N*M, total) }
}

func TestAccount_BalanceAndLedger(t *testing.T) {
    _, h, userID, cash, income := setup(t)
    // Make two entries: +1000 and +500 to cash
    mk := func(amt int64) {
        body := map[string]any{
            "user_id": userID.String(),
            "date": time.Now().UTC().Format(time.RFC3339),
            "currency": "USD",
            "category": "transfers",
            "lines": []map[string]any{
                {"account_id": cash.ID.String(), "side": "debit", "amount_minor": amt},
                {"account_id": income.ID.String(), "side": "credit", "amount_minor": amt},
            },
        }
        b, _ := json.Marshal(body)
        r := httptest.NewRequest(http.MethodPost, "/v1/entries", bytes.NewReader(b))
        r.Header.Set("Content-Type", "application/json")
        rr := httptest.NewRecorder(); h.ServeHTTP(rr, r)
        if rr.Code != http.StatusCreated { t.Fatalf("create failed: %d", rr.Code) }
    }
    mk(1000); mk(500)

    // Balance should be 1500
    rb := httptest.NewRequest(http.MethodGet, "/accounts/"+cash.ID.String()+"/balance?user_id="+userID.String(), nil)
    rbr := httptest.NewRecorder(); h.ServeHTTP(rbr, rb)
    if rbr.Code != http.StatusOK { t.Fatalf("balance expected 200, got %d", rbr.Code) }
    var br struct{ BalanceMinor int64 `json:"balance_minor"` }
    _ = json.Unmarshal(rbr.Body.Bytes(), &br)
    if br.BalanceMinor != 1500 { t.Fatalf("unexpected balance: %d", br.BalanceMinor) }

    // Ledger with limit=1 should return next_cursor
    rl := httptest.NewRequest(http.MethodGet, "/accounts/"+cash.ID.String()+"/ledger?user_id="+userID.String()+"&limit=1", nil)
    rlr := httptest.NewRecorder(); h.ServeHTTP(rlr, rl)
    if rlr.Code != http.StatusOK { t.Fatalf("ledger expected 200, got %d", rlr.Code) }
    var l1 struct{ Items []map[string]any `json:"items"`; NextCursor *string `json:"next_cursor"` }
    _ = json.Unmarshal(rlr.Body.Bytes(), &l1)
    if len(l1.Items) != 1 || l1.NextCursor == nil { t.Fatalf("expected 1 item and next_cursor; got: %+v", l1) }
}

func TestAccounts_InvalidAndSystemGuards(t *testing.T) {
    store, h, userID, _, _ := setup(t)

    // missing fields -> 400
    bad := map[string]any{"user_id": userID.String(), "name": "", "currency": "", "type": "asset"}
    bb, _ := json.Marshal(bad)
    r := httptest.NewRequest(http.MethodPost, "/v1/accounts", bytes.NewReader(bb))
    r.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    h.ServeHTTP(rr, r)
    if rr.Code != http.StatusBadRequest { t.Fatalf("expected 400 for invalid account, got %d", rr.Code) }

    // create a normal account
    acct := map[string]any{"user_id": userID.String(), "name": "Sys", "currency": "USD", "type": "asset", "method": "Bank", "vendor": "X"}
    ab, _ := json.Marshal(acct)
    r2 := httptest.NewRequest(http.MethodPost, "/v1/accounts", bytes.NewReader(ab))
    r2.Header.Set("Content-Type", "application/json")
    rr2 := httptest.NewRecorder()
    h.ServeHTTP(rr2, r2)
    if rr2.Code != http.StatusCreated { t.Fatalf("expected 201, got %d: %s", rr2.Code, rr2.Body.String()) }
    var ar acctResp
    _ = json.Unmarshal(rr2.Body.Bytes(), &ar)

    // mark as system in store and try to patch/delete
    aid := uuid.MustParse(ar.ID)
    a, _ := store.AccountByID(nil, userID, aid)
    a.System = true
    store.UpdateAccount(nil, a)

    // patch -> 403
    up := map[string]any{"name": "Noop"}
    ub, _ := json.Marshal(up)
    p := httptest.NewRequest(http.MethodPatch, "/v1/accounts/"+ar.ID+"?user_id="+userID.String(), bytes.NewReader(ub))
    p.Header.Set("Content-Type", "application/json")
    rp := httptest.NewRecorder()
    h.ServeHTTP(rp, p)
    if rp.Code != http.StatusForbidden { t.Fatalf("expected 403 for system account patch, got %d", rp.Code) }

    // delete -> 403
    d := httptest.NewRequest(http.MethodDelete, "/v1/accounts/"+ar.ID+"?user_id="+userID.String(), nil)
    rd := httptest.NewRecorder()
    h.ServeHTTP(rd, d)
    if rd.Code != http.StatusForbidden { t.Fatalf("expected 403 for system account delete, got %d", rd.Code) }
}

func TestAccounts_CreateDuplicatePathAndFilters(t *testing.T) {
    store, h, userID, _, _ := setup(t)

    // create account
    acct := map[string]any{
        "user_id":  userID.String(),
        "name":     "Monzo Current",
        "currency": "USD",
        "type":     "asset",
        "method":   "Bank",
        "vendor":   "Monzo",
    }
    b, _ := json.Marshal(acct)
    req := httptest.NewRequest(http.MethodPost, "/v1/accounts", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()
    h.ServeHTTP(rec, req)
    if rec.Code != http.StatusCreated {
        t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
    }
    var ar acctResp
    if err := json.Unmarshal(rec.Body.Bytes(), &ar); err != nil {
        t.Fatalf("decode: %v", err)
    }
    if ar.Path != "asset:bank:monzo" {
        t.Fatalf("unexpected path: %s", ar.Path)
    }

    // duplicate path -> 409
    rec = httptest.NewRecorder()
    reqDup := httptest.NewRequest(http.MethodPost, "/v1/accounts", bytes.NewReader(b))
    reqDup.Header.Set("Content-Type", "application/json")
    h.ServeHTTP(rec, reqDup)
    if rec.Code != http.StatusConflict {
        t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
    }

    // list with filters
    r2 := httptest.NewRequest(http.MethodGet, "/v1/accounts?user_id="+userID.String()+"&method=bank&vendor=monzo&type=asset", nil)
    rec2 := httptest.NewRecorder()
    h.ServeHTTP(rec2, r2)
    if rec2.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d: %s", rec2.Code, rec2.Body.String())
    }
    var arr []acctResp
    if err := json.Unmarshal(rec2.Body.Bytes(), &arr); err != nil {
        t.Fatalf("decode: %v", err)
    }
    if len(arr) != 1 || arr[0].Path != "asset:bank:monzo" {
        t.Fatalf("unexpected accounts filter result: %+v", arr)
    }

    // patch (rename path + metadata)
    up := map[string]any{"method": "Banking", "vendor": "Monzo"}
    ub, _ := json.Marshal(up)
    r3 := httptest.NewRequest(http.MethodPatch, "/v1/accounts/"+ar.ID+"?user_id="+userID.String(), bytes.NewReader(ub))
    r3.Header.Set("Content-Type", "application/json")
    rec3 := httptest.NewRecorder()
    h.ServeHTTP(rec3, r3)
    if rec3.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d: %s", rec3.Code, rec3.Body.String())
    }
    var ar2 acctResp
    json.Unmarshal(rec3.Body.Bytes(), &ar2)
    if ar2.Path != "asset:banking:monzo" {
        t.Fatalf("unexpected updated path: %s", ar2.Path)
    }

    // delete (soft)
    r4 := httptest.NewRequest(http.MethodDelete, "/v1/accounts/"+ar.ID+"?user_id="+userID.String(), nil)
    rec4 := httptest.NewRecorder()
    h.ServeHTTP(rec4, r4)
    if rec4.Code != http.StatusNoContent {
        t.Fatalf("expected 204, got %d: %s", rec4.Code, rec4.Body.String())
    }
    // verify inactive via repository
    a, err := store.AccountByID(nil, userID, uuid.MustParse(ar.ID))
    if err != nil {
        t.Fatalf("AccountByID: %v", err)
    }
    if a.Active != false {
        t.Fatalf("expected active=false after delete; got: %+v", a)
    }
}

func TestOpeningBalances_EndpointCreatesPerCurrency(t *testing.T) {
    _, h, userID, _, _ := setup(t)

    // Request GBP OpeningBalances (should create it)
    r := httptest.NewRequest(http.MethodGet, "/v1/accounts/opening-balances?user_id="+userID.String()+"&currency=GBP", nil)
    rr := httptest.NewRecorder(); h.ServeHTTP(rr, r)
    if rr.Code != http.StatusOK { t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String()) }
    var ar acctResp
    _ = json.Unmarshal(rr.Body.Bytes(), &ar)
    if ar.Type != "equity" || ar.Path != "equity:openingbalances" || ar.Currency != "GBP" {
        t.Fatalf("unexpected opening account: %+v", ar)
    }

    // Idempotent: second call returns same currency/system account
    rr2 := httptest.NewRecorder(); h.ServeHTTP(rr2, r)
    if rr2.Code != http.StatusOK { t.Fatalf("expected 200, got %d", rr2.Code) }
}

func TestAccounts_BatchCreate_MixedResults(t *testing.T) {
    _, h, userID, _, _ := setup(t)

    // First item valid, second item duplicate (same path/currency), third invalid (missing fields)
    items := []map[string]any{
        {"user_id": userID.String(), "name": "Wallet 1", "currency": "USD", "type": "asset", "method": "Cash", "vendor": "Pocket"},
        {"user_id": userID.String(), "name": "Wallet 2", "currency": "USD", "type": "asset", "method": "Cash", "vendor": "Pocket"},
        {"user_id": userID.String(), "name": "", "currency": "", "type": "asset", "method": "", "vendor": ""},
    }
    b, _ := json.Marshal(items)
    r := httptest.NewRequest(http.MethodPost, "/v1/accounts/batch", bytes.NewReader(b))
    r.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder(); h.ServeHTTP(rr, r)
    if rr.Code != http.StatusOK { t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String()) }
    var res []struct {
        Index   int        `json:"index"`
        Account *acctResp  `json:"account"`
        Error   string     `json:"error"`
        Code    string     `json:"code"`
    }
    _ = json.Unmarshal(rr.Body.Bytes(), &res)
    if len(res) != 3 { t.Fatalf("expected 3 results, got %d", len(res)) }
    if res[0].Account == nil || res[0].Error != "" { t.Fatalf("first should succeed: %+v", res[0]) }
    if res[1].Account != nil || res[1].Code == "" { t.Fatalf("second should fail with conflict: %+v", res[1]) }
    if res[2].Account != nil || res[2].Code != "invalid" { t.Fatalf("third should be invalid: %+v", res[2]) }
}
