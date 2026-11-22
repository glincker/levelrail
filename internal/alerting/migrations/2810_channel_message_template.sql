-- Optional message text with {{app}}, {{url}} style variables; empty keeps the built in message.
ALTER TABLE notification_channels ADD COLUMN message_template TEXT NOT NULL DEFAULT '';
