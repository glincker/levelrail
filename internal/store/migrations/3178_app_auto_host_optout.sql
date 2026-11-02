-- Apps that opted out of the automatic fallback hostname and the certificate
-- it would order. No row means the app keeps the current behavior.
CREATE TABLE app_auto_host_optout (
    app_name TEXT PRIMARY KEY
);
