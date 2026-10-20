-- Beta features an operator switched on from Settings, on top of the
-- APP_EXPERIMENTAL environment variable. Only features the platform marks
-- toggleable are read from here.
CREATE TABLE beta_feature_settings (
    feature    TEXT PRIMARY KEY,
    enabled    INTEGER NOT NULL DEFAULT 0,
    actor      TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);
