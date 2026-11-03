-- +goose Up
CREATE TABLE accounts (
    id TEXT COLLATE "C" PRIMARY KEY,
    balance BIGINT NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE accounts;
