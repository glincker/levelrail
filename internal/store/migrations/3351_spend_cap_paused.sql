-- Whether the cap controller stopped the app's containers this period: '' none, 'paused' stopped by the cap, 'resumed' started by hand.
ALTER TABLE app_spend_cap_state ADD COLUMN stop_state TEXT NOT NULL DEFAULT '';
