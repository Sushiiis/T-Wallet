-- migrations/000001_init.up.sql
-- gen_random_uuid() входит в ядро PostgreSQL 13+, расширение не нужно.

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE accounts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id),
    currency   TEXT NOT NULL DEFAULT 'RUB',
    balance    BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0), -- в копейках
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE transactions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type            TEXT NOT NULL,   -- deposit | withdraw | transfer
    status          TEXT NOT NULL,   -- completed | failed
    amount          BIGINT NOT NULL CHECK (amount > 0),
    from_account_id UUID REFERENCES accounts(id),
    to_account_id   UUID REFERENCES accounts(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ledger_entries (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES transactions(id),
    account_id     UUID NOT NULL REFERENCES accounts(id),
    amount         BIGINT NOT NULL, -- знаковое: >0 зачисление, <0 списание
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE idempotency_keys (
    key            TEXT PRIMARY KEY,
    transaction_id UUID REFERENCES transactions(id),
    request_hash   TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic      TEXT NOT NULL,
    payload    JSONB NOT NULL,
    published  BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);


CREATE INDEX idx_accounts_user_id           ON accounts(user_id);
CREATE INDEX idx_ledger_entries_account_id  ON ledger_entries(account_id);
CREATE INDEX idx_transactions_from_account  ON transactions(from_account_id);
CREATE INDEX idx_transactions_to_account    ON transactions(to_account_id);
CREATE INDEX idx_outbox_unpublished         ON outbox(created_at) WHERE published = false;