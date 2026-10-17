-- Certificates whose private key the control plane generated (enrolled
-- before agents made their own keys) must be re-enrolled, never renewed.
-- Nodes with no certificate carry an empty origin.
UPDATE nodes SET cert_key_origin = 'reenroll_required' WHERE cert_key_origin = 'server' AND cert_fingerprint != '';
UPDATE nodes SET cert_key_origin = '' WHERE cert_key_origin = 'server';
