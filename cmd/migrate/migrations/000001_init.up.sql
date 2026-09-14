DROP TABLE IF EXISTS auth_session_events;
DROP TABLE IF EXISTS refresh_sessions;
DROP TABLE IF EXISTS users;

CREATE TABLE users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    remark TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    role TEXT NOT NULL DEFAULT 'user',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_role_check CHECK (role IN ('admin', 'user'))
);
