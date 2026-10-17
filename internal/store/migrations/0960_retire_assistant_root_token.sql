-- Early betas gave the assistant a persistent root token; turns now mint
-- per-sender tokens. Revoke the old token and drop its stored secret.
UPDATE api_tokens SET revoked_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE name = 'AI Assistant (internal)' AND owner_user_id = '' AND revoked_at IS NULL;
DELETE FROM service_secret_values WHERE service_name = 'ai-assistant-internal-token';
DELETE FROM service_secrets WHERE service_name = 'ai-assistant-internal-token';
