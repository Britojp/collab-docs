CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT UNIQUE NOT NULL,
    name          TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE documents (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title        TEXT NOT NULL,
    owner_id     UUID REFERENCES users(id),
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW(),
    snapshot_url TEXT
);

CREATE TABLE doc_permissions (
    doc_id  UUID REFERENCES documents(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id)     ON DELETE CASCADE,
    role    TEXT CHECK (role IN ('owner', 'editor', 'viewer')),
    PRIMARY KEY (doc_id, user_id)
);

CREATE TABLE operations (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    doc_id       UUID REFERENCES documents(id) ON DELETE CASCADE,
    user_id      UUID REFERENCES users(id),
    type         TEXT CHECK (type IN ('insert', 'delete')),
    position     INT NOT NULL,
    character    CHAR,
    vector_clock JSONB NOT NULL,
    created_at   TIMESTAMPTZ DEFAULT NOW()
) PARTITION BY HASH (doc_id);

CREATE TABLE operations_p0 PARTITION OF operations FOR VALUES WITH (MODULUS 8, REMAINDER 0);
CREATE TABLE operations_p1 PARTITION OF operations FOR VALUES WITH (MODULUS 8, REMAINDER 1);
CREATE TABLE operations_p2 PARTITION OF operations FOR VALUES WITH (MODULUS 8, REMAINDER 2);
CREATE TABLE operations_p3 PARTITION OF operations FOR VALUES WITH (MODULUS 8, REMAINDER 3);
CREATE TABLE operations_p4 PARTITION OF operations FOR VALUES WITH (MODULUS 8, REMAINDER 4);
CREATE TABLE operations_p5 PARTITION OF operations FOR VALUES WITH (MODULUS 8, REMAINDER 5);
CREATE TABLE operations_p6 PARTITION OF operations FOR VALUES WITH (MODULUS 8, REMAINDER 6);
CREATE TABLE operations_p7 PARTITION OF operations FOR VALUES WITH (MODULUS 8, REMAINDER 7);

CREATE TABLE audit_log (
    id         BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    user_id    UUID,
    doc_id     UUID,
    payload    JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_operations_doc_id ON operations (doc_id);
CREATE INDEX idx_audit_log_doc_id  ON audit_log  (doc_id);
CREATE INDEX idx_audit_log_user_id ON audit_log  (user_id);
