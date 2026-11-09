package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/theepsi/Ledgersito/internal/application/transfers"
	"github.com/theepsi/Ledgersito/internal/domain/account"
	"github.com/theepsi/Ledgersito/internal/domain/transfer"
)

type TransferService interface {
	Execute(context.Context, string, transfer.Request) (transfers.Response, error)
}

func NewRouter(accounts Service, transfers TransferService, history HistoryService, logger *slog.Logger) *Handler {
	h := New(accounts, logger)
	h.transfers = transfers
	h.history = history
	return h
}

var errInvalidTransferJSON = errors.New("invalid transfer JSON")

func decodeTransfer(body []byte) (transfer.Request, error) {
	fields, err := decodeObject(body, "source_account_id", "destination_account_id", "amount_sititos")
	if err != nil {
		return transfer.Request{}, errInvalidTransferJSON
	}
	source, err := decodeString(fields["source_account_id"])
	if err != nil {
		return transfer.Request{}, errInvalidTransferJSON
	}
	destination, err := decodeString(fields["destination_account_id"])
	if err != nil {
		return transfer.Request{}, errInvalidTransferJSON
	}
	raw := string(fields["amount_sititos"])
	if raw == "" || strings.ContainsAny(raw, ".eE") {
		return transfer.Request{}, errInvalidTransferJSON
	}
	for i, c := range raw {
		if c == '-' && i == 0 {
			continue
		}
		if c < '0' || c > '9' {
			return transfer.Request{}, errInvalidTransferJSON
		}
	}
	sititos, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return transfer.Request{}, transfer.ErrInvalidAmount
	}
	req := transfer.Request{SourceID: source, DestinationID: destination, Sititos: sititos}
	if err := req.Validate(); err != nil {
		return transfer.Request{}, err
	}
	return req, nil
}

func (h *Handler) serveTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		h.failure(w, 405, "method_not_allowed", "Method not allowed.")
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		h.failure(w, 400, "invalid_request", "Query parameters are not supported.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	body, key, ok := h.readPost(w, r)
	if !ok {
		return
	}
	req, err := decodeTransfer(body)
	if err != nil {
		h.transferError(w, ctx, err)
		return
	}
	reply, err := h.transfers.Execute(ctx, key, req)
	if err != nil {
		h.transferError(w, ctx, err)
		return
	}
	if ctx.Err() != nil {
		h.transferError(w, ctx, ctx.Err())
		return
	}
	h.write(w, reply.Status, reply.Body)
}

func (h *Handler) transferError(w http.ResponseWriter, ctx context.Context, err error) {
	h.logger.Error("transfer operation failed", "error", err)
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
		h.failure(w, 503, "request_timeout", "Operation timed out. Retry with the same idempotency key.")
	case errors.Is(err, errInvalidTransferJSON):
		h.failure(w, 400, "invalid_request", "Provide source_account_id, destination_account_id, and integer amount_sititos.")
	case errors.Is(err, account.ErrInvalidID):
		h.failure(w, 400, "invalid_account_id", "Account IDs must be nonempty and contain no whitespace.")
	case errors.Is(err, transfer.ErrInvalidAmount):
		h.failure(w, 400, "invalid_amount", "Amount must be positive int64 sititos.")
	case errors.Is(err, transfer.ErrSameAccount):
		h.failure(w, 400, "same_account", "Source and destination must differ.")
	case errors.Is(err, transfers.ErrKeyReused):
		h.failure(w, 422, "idempotency_key_reused", "Key was already used with different transfer details.")
	case errors.Is(err, transfers.ErrNotInitialized):
		h.failure(w, 409, "ledger_not_initialized", "Initialize the ledger before transferring.")
	case errors.Is(err, transfers.ErrAccountNotFound):
		h.failure(w, 404, "account_not_found", "Source or destination account does not exist.")
	case errors.Is(err, transfer.ErrNotTransferable):
		h.failure(w, 422, "account_not_transferable", "Origin cannot participate and reservoir cannot receive transfers.")
	case errors.Is(err, transfer.ErrInsufficientFunds):
		h.failure(w, 422, "insufficient_funds", "Source account has insufficient funds.")
	case errors.Is(err, transfer.ErrBalanceOverflow):
		h.failure(w, 422, "balance_overflow", "Resulting balance exceeds int64 range.")
	default:
		h.failure(w, 500, "internal_error", "An internal error occurred.")
	}
}
