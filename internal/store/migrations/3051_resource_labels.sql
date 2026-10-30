-- Free-form operator labels for apps, databases and stacks; nodes keep node_labels.
CREATE TABLE resource_labels (
	kind TEXT NOT NULL,
	name TEXT NOT NULL,
	key TEXT NOT NULL,
	value TEXT NOT NULL,
	PRIMARY KEY (kind, name, key)
);
