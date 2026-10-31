-- Team owned mail domains and mailboxes (ADR 059 section 2.5). Metadata only:
-- passwords live in the mail engine and message content never reaches this table.
CREATE TABLE mail_domains (
    domain      TEXT    PRIMARY KEY,
    service     TEXT    NOT NULL,
    team_id     TEXT    NOT NULL,
    created_by  TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL
);

CREATE TABLE mail_mailboxes (
    address         TEXT    PRIMARY KEY,
    domain          TEXT    NOT NULL REFERENCES mail_domains(domain),
    localpart       TEXT    NOT NULL,
    service         TEXT    NOT NULL,
    team_id         TEXT    NOT NULL,
    linked_user_id  TEXT    NOT NULL DEFAULT '',
    suspended       INTEGER NOT NULL DEFAULT 0,
    created_by      TEXT    NOT NULL DEFAULT '',
    created_at      TEXT    NOT NULL,
    updated_at      TEXT    NOT NULL
);

CREATE INDEX mail_mailboxes_team ON mail_mailboxes (team_id, domain);
CREATE INDEX mail_mailboxes_linked_user ON mail_mailboxes (linked_user_id) WHERE linked_user_id <> '';
