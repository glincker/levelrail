-- Builder tier a join token enrols its node with (server, managed or personal).
ALTER TABLE node_join_tokens ADD COLUMN build_trust TEXT NOT NULL DEFAULT 'server';
