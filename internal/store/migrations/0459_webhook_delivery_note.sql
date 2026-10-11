-- Outcome note for a delivery that was accepted but did not deploy (an
-- ignored branch, a path filter skip, a queued or held deploy), so the
-- Source page and CLI can say why a green 200 produced no deploy. Empty
-- for deliveries that returned an error; those keep using error.
ALTER TABLE webhook_deliveries ADD COLUMN note TEXT NOT NULL DEFAULT '';
