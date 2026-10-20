-- A network share is an external NFS or CIFS/SMB export this control
-- plane can hand to Docker's own `local` volume driver (type=nfs /
-- type=cifs passthrough), the same no-plaintext-credential split
-- migrations/0046_registry_credentials.sql established: a CIFS share's
-- password goes through internal/secrets, keyed by
-- "network-share/<id>", never a column on this table.
CREATE TABLE network_shares (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    protocol      TEXT NOT NULL,
    host          TEXT NOT NULL,
    remote_path   TEXT NOT NULL,
    mount_options TEXT NOT NULL DEFAULT '',
    username      TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL
);
