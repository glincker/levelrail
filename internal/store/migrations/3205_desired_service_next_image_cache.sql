-- Per app override of app.yaml next.image_cache (persistent or none); empty defers to the spec.
ALTER TABLE desired_services ADD COLUMN next_image_cache TEXT NOT NULL DEFAULT '';
