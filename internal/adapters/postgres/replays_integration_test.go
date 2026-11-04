//go:build integration

package postgres_test

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/theepsi/Ledgersito/internal/adapters/httpapi"
	"github.com/theepsi/Ledgersito/internal/adapters/postgres"
	"github.com/theepsi/Ledgersito/internal/application/accounts"
	"github.com/theepsi/Ledgersito/internal/application/createaccount"
)

type fixtureGenerator struct {
	t      *testing.T
	conn   *pgx.Conn
	prefix string
	calls  int
}

func (g *fixtureGenerator) Generate() (string, error) {
	g.calls++
	id := g.prefix + rand.Text()
	fixture(g.t, g.conn, id)
	return id, nil
}

func TestReplayStorageIntegration(t *testing.T) {
	conn, ctx := testDB(t)
	store := postgres.NewAPIStore(conn.Config())
	prefix := "replay-" + rand.Text()
	newKey := func(t *testing.T) string {
		t.Helper()
		key := prefix + rand.Text()
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := conn.Exec(cleanup, "DELETE FROM account_creation_replays WHERE key = $1", key); err != nil {
				t.Error(err)
			}
		})
		return key
	}
	makeReply := func(id string) func(createaccount.Store) (accounts.Response, error) {
		return func(txStore createaccount.Store) (accounts.Response, error) {
			a := fixture(t, conn, id)
			if err := txStore.Create(ctx, a); err != nil {
				return accounts.Response{}, err
			}
			return accounts.Response{Status: 201, Body: []byte(`{"id":"` + id + `"}`)}, nil
		}
	}

	t.Run("replay and conflict", func(t *testing.T) {
		key := newKey(t)
		id := prefix + "-original"
		first, err := store.CreateOnce(ctx, key, "Savings", makeReply(id))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, "UPDATE accounts SET balance = 12 WHERE id = $1", id); err != nil {
			t.Fatal(err)
		}
		never := func(createaccount.Store) (accounts.Response, error) {
			t.Fatal("replay/conflict attempted account creation")
			return accounts.Response{}, nil
		}
		replay, err := store.CreateOnce(ctx, key, "Savings", never)
		if err != nil || string(first.Body) != string(replay.Body) || replay.Status != 201 {
			t.Fatalf("replay changed: %+v, %v", replay, err)
		}
		if _, err := store.CreateOnce(ctx, key, "savings", never); !errors.Is(err, accounts.ErrKeyReused) {
			t.Fatalf("expected key conflict, got %v", err)
		}
	})

	t.Run("rollback after insert releases key", func(t *testing.T) {
		key := newKey(t)
		id := prefix + "-rollback"
		failure := errors.New("failure after account insert")
		_, err := store.CreateOnce(ctx, key, "Savings", func(s createaccount.Store) (accounts.Response, error) {
			if _, err := makeReply(id)(s); err != nil {
				return accounts.Response{}, err
			}
			return accounts.Response{}, failure
		})
		if !errors.Is(err, failure) {
			t.Fatal(err)
		}
		var count int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM accounts WHERE id = $1", id).Scan(&count); err != nil || count != 0 {
			t.Fatalf("account survived rollback: %d, %v", count, err)
		}
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM account_creation_replays WHERE key = $1", key).Scan(&count); err != nil || count != 0 {
			t.Fatalf("key survived rollback: %d, %v", count, err)
		}
		if _, err := store.CreateOnce(ctx, key, "Another name", makeReply(id)); err != nil {
			t.Fatalf("rolled-back key cannot be reused: %v", err)
		}
	})

	t.Run("concurrent same key", func(t *testing.T) {
		key := newKey(t)
		id := prefix + "-concurrent"
		a := fixture(t, conn, id)
		start := make(chan struct{})
		type result struct {
			reply accounts.Response
			err   error
		}
		results := make(chan result, 10)
		for range 10 {
			go func() {
				<-start
				reply, err := store.CreateOnce(ctx, key, "Savings", func(s createaccount.Store) (accounts.Response, error) {
					if err := s.Create(ctx, a); err != nil {
						return accounts.Response{}, err
					}
					return accounts.Response{Status: 201, Body: []byte(`{"created":true}`)}, nil
				})
				results <- result{reply, err}
			}()
		}
		close(start)
		for range 10 {
			got := <-results
			if got.err != nil || got.reply.Status != 201 || string(got.reply.Body) != `{"created":true}` {
				t.Errorf("unexpected concurrent result: %+v", got)
			}
		}
		var count int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM accounts WHERE id = $1", id).Scan(&count); err != nil || count != 1 {
			t.Fatalf("expected one account: %d %v", count, err)
		}
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM account_creation_replays WHERE key = $1 AND status_code = 201", key).Scan(&count); err != nil || count != 1 {
			t.Fatalf("expected one complete replay: %d %v", count, err)
		}
	})

	t.Run("wait timeout does not disturb owner", func(t *testing.T) {
		key := newKey(t)
		a := fixture(t, conn, prefix+"-waiting")
		entered, release := make(chan struct{}), make(chan struct{})
		done := make(chan error, 1)
		go func() {
			_, err := store.CreateOnce(ctx, key, "Savings", func(s createaccount.Store) (accounts.Response, error) {
				if err := s.Create(ctx, a); err != nil {
					close(entered)
					return accounts.Response{}, err
				}
				close(entered)
				<-release
				return accounts.Response{Status: 201, Body: []byte(`{"created":true}`)}, nil
			})
			done <- err
		}()
		<-entered
		waitCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		_, err := store.CreateOnce(waitCtx, key, "Savings", func(createaccount.Store) (accounts.Response, error) {
			return accounts.Response{}, errors.New("waiter must not create")
		})
		cancel()
		close(release)
		ownerErr := <-done
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected wait deadline, got %v", err)
		}
		if ownerErr != nil {
			t.Fatalf("owner disturbed by waiter: %v", ownerErr)
		}
	})

	t.Run("competing payloads", func(t *testing.T) {
		key := newKey(t)
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, name := range []string{"First", "Second"} {
			a := fixture(t, conn, prefix+"-"+name)
			go func() {
				<-start
				_, err := store.CreateOnce(ctx, key, name, func(s createaccount.Store) (accounts.Response, error) {
					if err := s.Create(ctx, a); err != nil {
						return accounts.Response{}, err
					}
					return accounts.Response{Status: 201, Body: []byte(`{"created":true}`)}, nil
				})
				results <- err
			}()
		}
		close(start)
		successes, conflicts := 0, 0
		for range 2 {
			switch err := <-results; {
			case err == nil:
				successes++
			case errors.Is(err, accounts.ErrKeyReused):
				conflicts++
			default:
				t.Errorf("unexpected result: %v", err)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf("expected one success and one key conflict: %d %d", successes, conflicts)
		}
	})

	t.Run("HTTP replay and nonunique names", func(t *testing.T) {
		generator := &fixtureGenerator{t: t, conn: conn, prefix: prefix}
		handler := httpapi.New(accounts.New(generator, store), slog.New(slog.NewTextHandler(io.Discard, nil)))
		key := newKey(t)
		post := func(key, body string) *httptest.ResponseRecorder {
			req := httptest.NewRequest("POST", "/accounts", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", key)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			return w
		}
		if invalid := post(key, `{"name":" "}`); invalid.Code != 400 {
			t.Fatal("invalid request accepted")
		}
		first := post(key, `{"name":" Savings "}`)
		replay := post(key, "{ \"name\" : \"Savings\" }")
		if first.Code != 201 || replay.Code != 201 || first.Body.String() != replay.Body.String() {
			t.Fatalf("HTTP replay mismatch: %d %s / %d %s", first.Code, first.Body, replay.Code, replay.Body)
		}
		if conflict := post(key, `{"name":"savings"}`); conflict.Code != 422 {
			t.Fatalf("expected conflict: %d %s", conflict.Code, conflict.Body)
		}
		second := post(newKey(t), `{"name":"Savings"}`)
		if second.Code != 201 || second.Body.String() == first.Body.String() || generator.calls != 2 {
			t.Fatal("different keys must allow separate accounts with equal names")
		}
		list := httptest.NewRecorder()
		handler.ServeHTTP(list, httptest.NewRequest("GET", "/accounts", nil))
		if list.Code != 200 || !strings.Contains(list.Body.String(), `"name":"Savings"`) {
			t.Fatalf("listing failed: %d %s", list.Code, list.Body)
		}
	})

	t.Run("listing preserves balances and breaks name ties", func(t *testing.T) {
		for _, suffix := range []string{"-list-b", "-list-a"} {
			a := fixture(t, conn, prefix+suffix)
			if err := postgres.NewAccountStore(conn).Create(ctx, a); err != nil {
				t.Fatal(err)
			}

		}
		id := prefix + "-list-a"
		if _, err := conn.Exec(ctx, "UPDATE accounts SET balance = -7 WHERE id = $1", id); err != nil {
			t.Fatal(err)
		}
		items, err := store.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.IsSortedFunc(items, func(a, b accounts.Account) int {
			if a.Name != b.Name {
				if a.Name < b.Name {
					return -1
				}
				return 1
			}
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		}) {
			t.Fatal("listing is not ordered by name then ID")
		}
		found := false
		for _, item := range items {
			if item.ID == id {
				found = item.Name == "Savings" && item.Balance.Sititos == -7 && item.Balance.Currency == "SITO"
			}
		}
		if !found {
			t.Fatal("missing or incorrect persisted account")
		}
	})
}
