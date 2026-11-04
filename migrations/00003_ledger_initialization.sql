-- +goose Up
ALTER TABLE accounts ADD COLUMN role TEXT NOT NULL DEFAULT 'ordinary';
ALTER TABLE accounts ADD CONSTRAINT accounts_role CHECK (role IN ('ordinary', 'reservoir', 'origin'));
ALTER TABLE accounts ADD CONSTRAINT accounts_nonnegative CHECK (role = 'origin' OR balance >= 0);
CREATE UNIQUE INDEX one_system_account_per_role ON accounts (role) WHERE role <> 'ordinary';

CREATE TABLE ledger_transactions (
    id TEXT COLLATE "C" PRIMARY KEY,
    reference TEXT NOT NULL,
    posted BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE entries (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES ledger_transactions(id),
    account_id TEXT NOT NULL REFERENCES accounts(id),
    amount BIGINT NOT NULL CHECK (amount <> 0)
);
CREATE INDEX entries_account ON entries (account_id);
CREATE INDEX entries_transaction ON entries (transaction_id);

CREATE TABLE ledger_initialization (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    supply BIGINT NOT NULL CHECK (supply = 100000000),
    opening_transaction_id TEXT NOT NULL UNIQUE REFERENCES ledger_transactions(id)
);

-- +goose StatementBegin
CREATE FUNCTION reject_history_change() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'ledger history is immutable' USING ERRCODE = '23514';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER entries_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON entries
FOR EACH STATEMENT EXECUTE FUNCTION reject_history_change();
CREATE TRIGGER initialization_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON ledger_initialization
FOR EACH STATEMENT EXECUTE FUNCTION reject_history_change();
CREATE TRIGGER transactions_no_truncate BEFORE TRUNCATE ON ledger_transactions
FOR EACH STATEMENT EXECUTE FUNCTION reject_history_change();

-- +goose StatementBegin
CREATE FUNCTION protect_account_identity() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id OR NEW.role IS DISTINCT FROM OLD.role THEN
        RAISE EXCEPTION 'account identity and role are immutable' USING ERRCODE = '23514';
    END IF;
    IF OLD.role = 'origin' AND NEW.balance IS DISTINCT FROM OLD.balance THEN
        RAISE EXCEPTION 'origin balance is fixed' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER account_identity_immutable BEFORE UPDATE ON accounts
FOR EACH ROW EXECUTE FUNCTION protect_account_identity();

-- +goose StatementBegin
CREATE FUNCTION protect_posted_transaction() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.posted THEN
        RAISE EXCEPTION 'posted transactions are immutable' USING ERRCODE = '23514';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    IF NEW.id IS DISTINCT FROM OLD.id THEN
        RAISE EXCEPTION 'transaction identity is immutable' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER posted_transaction_immutable BEFORE UPDATE OR DELETE ON ledger_transactions
FOR EACH ROW EXECUTE FUNCTION protect_posted_transaction();

-- +goose StatementBegin
CREATE FUNCTION check_entry_insert() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    is_posted BOOLEAN;
BEGIN
    SELECT posted INTO is_posted FROM ledger_transactions WHERE id = NEW.transaction_id FOR UPDATE;
    IF NOT FOUND OR is_posted THEN
        RAISE EXCEPTION 'entries require an existing unposted transaction' USING ERRCODE = '23514';
    END IF;
    IF NEW.transaction_id <> 'system-opening' AND
       EXISTS (SELECT 1 FROM accounts WHERE id = NEW.account_id AND role = 'origin') THEN
        RAISE EXCEPTION 'origin is not a transfer account' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER entry_requires_open_transaction BEFORE INSERT ON entries
FOR EACH ROW EXECUTE FUNCTION check_entry_insert();

-- +goose StatementBegin
CREATE FUNCTION check_balanced_transaction() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    is_posted BOOLEAN;
    entry_count BIGINT;
    total NUMERIC;
BEGIN
    SELECT posted INTO is_posted FROM ledger_transactions WHERE id = NEW.id;
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    SELECT count(*), coalesce(sum(amount), 0) INTO entry_count, total
    FROM entries WHERE transaction_id = NEW.id;
    IF NOT is_posted OR entry_count < 2 OR total <> 0 THEN
        RAISE EXCEPTION 'transaction must be posted with at least two balanced entries' USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER transaction_must_balance AFTER INSERT OR UPDATE ON ledger_transactions
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_balanced_transaction();

-- +goose StatementBegin
CREATE FUNCTION initialize_ledger() RETURNS BOOLEAN LANGUAGE plpgsql AS $$
DECLARE
    initialized BOOLEAN;
    stored_supply BIGINT;
    opening_id TEXT;
BEGIN
    -- Serialize initializers across processes and exclude concurrent account
    -- writes while validating the opening state. SELECT listings remain available.
    LOCK TABLE accounts IN SHARE ROW EXCLUSIVE MODE;
    SELECT supply, opening_transaction_id INTO stored_supply, opening_id FROM ledger_initialization;
    initialized := FOUND;

    IF initialized THEN
        IF stored_supply <> 100000000 OR opening_id <> 'system-opening' THEN
            RAISE EXCEPTION 'conflicting initialization record' USING ERRCODE = '23514';
        END IF;
        IF (SELECT count(*) FROM accounts WHERE role = 'origin' AND id = 'system-origin'
            AND name = 'Supply origin' AND balance = -100000000) <> 1
           OR (SELECT count(*) FROM accounts WHERE role = 'reservoir' AND id = 'system-reservoir'
            AND name = 'Reservoir') <> 1 THEN
            RAISE EXCEPTION 'inconsistent system accounts' USING ERRCODE = '23514';
        END IF;
        IF (SELECT count(*) FROM ledger_transactions WHERE id = opening_id
            AND reference = 'initial supply' AND posted) <> 1
           OR (SELECT count(*) FROM entries WHERE transaction_id = opening_id) <> 2
           OR (SELECT count(*) FROM entries WHERE transaction_id = opening_id
            AND account_id = 'system-origin' AND amount = -100000000) <> 1
           OR (SELECT count(*) FROM entries WHERE transaction_id = opening_id
            AND account_id = 'system-reservoir' AND amount = 100000000) <> 1 THEN
            RAISE EXCEPTION 'inconsistent opening history' USING ERRCODE = '23514';
        END IF;
        IF EXISTS (
            SELECT 1 FROM accounts a LEFT JOIN
                (SELECT account_id, sum(amount) AS total FROM entries GROUP BY account_id) e
                ON e.account_id = a.id
            WHERE a.balance <> coalesce(e.total, 0)
        ) OR EXISTS (
            SELECT 1 FROM ledger_transactions t LEFT JOIN entries e ON e.transaction_id = t.id
            GROUP BY t.id, t.posted HAVING NOT t.posted OR count(e.id) < 2 OR coalesce(sum(e.amount), 0) <> 0
        ) OR (SELECT coalesce(sum(balance), 0) FROM accounts WHERE role <> 'origin') <> 100000000 THEN
            RAISE EXCEPTION 'inconsistent ledger balances or history' USING ERRCODE = '23514';
        END IF;
        RETURN FALSE;
    END IF;

    IF EXISTS (SELECT 1 FROM accounts WHERE balance <> 0 OR role <> 'ordinary'
        OR id IN ('system-origin', 'system-reservoir'))
       OR EXISTS (SELECT 1 FROM ledger_transactions) OR EXISTS (SELECT 1 FROM entries) THEN
        RAISE EXCEPTION 'unexplained pre-initialization ledger state' USING ERRCODE = '23514';
    END IF;

    INSERT INTO accounts (id, name, role, balance) VALUES
        ('system-origin', 'Supply origin', 'origin', -100000000),
        ('system-reservoir', 'Reservoir', 'reservoir', 100000000);
    INSERT INTO ledger_transactions (id, reference) VALUES ('system-opening', 'initial supply');
    INSERT INTO entries (transaction_id, account_id, amount) VALUES
        ('system-opening', 'system-origin', -100000000),
        ('system-opening', 'system-reservoir', 100000000);
    UPDATE ledger_transactions SET posted = TRUE WHERE id = 'system-opening';
    INSERT INTO ledger_initialization (supply, opening_transaction_id) VALUES (100000000, 'system-opening');
    RETURN TRUE;
END;
$$;
-- +goose StatementEnd

-- +goose Down
DO $$ BEGIN RAISE EXCEPTION 'ledger history migration is forward-only; do not erase initialized history'; END $$;
