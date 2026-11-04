-- +goose Up
ALTER TABLE accounts ADD COLUMN name TEXT COLLATE "C";
UPDATE accounts SET name = 'Unnamed account';
ALTER TABLE accounts ALTER COLUMN name SET NOT NULL;
ALTER TABLE accounts ADD CONSTRAINT accounts_name_length CHECK (char_length(name) BETWEEN 1 AND 100);

CREATE TABLE account_creation_replays (
    key TEXT COLLATE "C" PRIMARY KEY,
    request_name TEXT NOT NULL,
    status_code INTEGER,
    response_body BYTEA,
    CONSTRAINT complete_response CHECK (
        (status_code IS NULL AND response_body IS NULL) OR
        (status_code IS NOT NULL AND status_code = 201 AND response_body IS NOT NULL)
    )
);

-- +goose Down
DROP TABLE account_creation_replays;
ALTER TABLE accounts DROP CONSTRAINT accounts_name_length;
ALTER TABLE accounts DROP COLUMN name;
