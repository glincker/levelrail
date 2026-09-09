-- desired_services was missing the domain list a service should be
-- reachable on, needed for ingress routing. This was a
-- real gap from the original translation (spec.Service.Domains was
-- never carried through into store.DesiredService), not a speculative
-- addition: the app.yaml example in the app spec declares domains on
-- every service, and there is a real consumer for this column right now.
ALTER TABLE desired_services ADD COLUMN domains TEXT NOT NULL DEFAULT '[]';
