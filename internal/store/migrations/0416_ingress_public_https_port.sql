-- Port clients use when a proxy fronts the ingress (0 = the ingress listen port),
-- and whether that proxy terminates TLS so this instance never runs ACME.
ALTER TABLE ingress_settings ADD COLUMN public_https_port INTEGER NOT NULL DEFAULT 0 CHECK (public_https_port BETWEEN 0 AND 65535);
ALTER TABLE ingress_settings ADD COLUMN tls_terminated_upstream INTEGER NOT NULL DEFAULT 0;
