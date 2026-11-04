-- +goose Up
CREATE TABLE transfer_replays (
    key TEXT COLLATE "C" PRIMARY KEY,
    source_account_id TEXT NOT NULL,
    destination_account_id TEXT NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    status_code INTEGER,
    response_body BYTEA,
    CONSTRAINT complete_transfer_response CHECK (
        (status_code IS NULL AND response_body IS NULL) OR
        (status_code IS NOT NULL AND status_code = 201 AND response_body IS NOT NULL)
    )
);

-- +goose StatementBegin
CREATE FUNCTION protect_reservoir_distribution() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.amount > 0 AND NEW.transaction_id <> 'system-opening' AND
       EXISTS (SELECT 1 FROM accounts WHERE id = NEW.account_id AND role = 'reservoir') THEN
        RAISE EXCEPTION 'reservoir cannot receive transfers' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER reservoir_distribution_only BEFORE INSERT ON entries
FOR EACH ROW EXECUTE FUNCTION protect_reservoir_distribution();

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'transfer migration is forward-only; preserve replay and ledger history'; END $$;
