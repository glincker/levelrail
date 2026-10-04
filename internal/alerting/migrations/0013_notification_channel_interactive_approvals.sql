-- interactive_secret verifies an inbound chat button click really came
-- from this channel's own app (Slack signing secret, or a Discord
-- application's Ed25519 public key): required only when
-- interactive_approvals is on.
ALTER TABLE notification_channels ADD COLUMN interactive_approvals INTEGER NOT NULL DEFAULT 0;
ALTER TABLE notification_channels ADD COLUMN interactive_secret TEXT NOT NULL DEFAULT '';
