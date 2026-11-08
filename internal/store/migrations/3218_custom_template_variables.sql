-- Variable names the operator fills in at deploy, as a JSON array; empty means none.
ALTER TABLE custom_templates ADD COLUMN variables TEXT NOT NULL DEFAULT '';
