-- +goose Up
ALTER TABLE ledger_transactions ADD COLUMN posted_at TIMESTAMPTZ;
ALTER TABLE ledger_transactions
    ADD CONSTRAINT ledger_transactions_posted_at_requires_posted
    CHECK (posted_at IS NULL OR posted);

-- +goose StatementBegin
CREATE FUNCTION stamp_transaction_posted_at() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.posted THEN
            NEW.posted_at := clock_timestamp();
        ELSE
            NEW.posted_at := NULL;
        END IF;
    ELSIF NOT OLD.posted AND NEW.posted THEN
        NEW.posted_at := clock_timestamp();
    ELSIF NOT NEW.posted THEN
        NEW.posted_at := NULL;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER stamp_transaction_posted_at
BEFORE INSERT OR UPDATE ON ledger_transactions
FOR EACH ROW EXECUTE FUNCTION stamp_transaction_posted_at();

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'posted_at migration is forward-only; preserve transaction posting history'; END $$;
