-- Extra demo rows capture-demo.sh applies after `demo seed`: release records
-- and the environment binding the app overview Releases card reads. Derived
-- from existing deploy attempts so the timestamps stay relative to the seed.
INSERT INTO releases (id, project_id, source_app, source_deploy_attempt_id, image_ref, image_tag, config, commit_sha, created_at, created_by)
SELECT 'rel_' || d.id, '', d.service_name, d.id,
       substr(d.image, 1, instr(d.image, ':') - 1) || '@sha256:' || substr(lower(hex(d.commit_sha || d.started_at || d.id)), 1, 64),
       '', '{}', d.commit_sha, d.started_at, 'alex'
FROM deploy_attempts d
WHERE d.service_name = 'marketing-site' AND d.status = 'succeeded'
ORDER BY d.started_at DESC LIMIT 4;

INSERT INTO environment_bindings (app, environment_id, release_id, state, previous_release_id, updated_at, updated_by)
SELECT 'marketing-site', 'env_production', r.id,
       'active', (SELECT id FROM releases WHERE source_app = 'marketing-site' ORDER BY created_at DESC LIMIT 1 OFFSET 1),
       r.created_at, 'alex'
FROM releases r WHERE r.source_app = 'marketing-site' ORDER BY r.created_at DESC LIMIT 1;

INSERT INTO environment_binding_history (app, release_id, changed_at, changed_by, reason)
SELECT source_app, id, created_at, 'alex', 'deploy' FROM releases WHERE source_app = 'marketing-site' ORDER BY created_at ASC;

-- One analysis report for the app overview Analysis card.
INSERT INTO analysis_reports (id, app, trigger_kind, trigger_id, status, schema_version, report, created_at)
SELECT 'ar_demo_1', 'marketing-site', 'alert', 'alert_demo_p95', 'complete', 1,
 '{"status":"complete","summary":"Latency rose after the latest deploy; memory climbed and one container restarted.","timeline":[{"at":"' || d.started_at || '","kind":"deploy","ref":"' || d.id || '","detail":"Deployed ' || d.commit_sha || '"},{"at":"' || d.started_at || '","kind":"metric","ref":"memory","detail":"Memory above 85 percent of the limit"},{"at":"' || d.started_at || '","kind":"restart","ref":"marketing-site","detail":"Container restarted once"}],"causes":[{"rank":1,"id":"c1","title":"Memory pressure after the new build","score":0.82,"confidence":"high","evidence":["Memory rose 14 percent within 20 minutes of the deploy","One restart followed the climb"]},{"rank":2,"id":"c2","title":"Cold cache after rollout","score":0.41,"confidence":"medium","evidence":["Cache hit ratio dipped for 10 minutes"]}],"recommendations":[{"id":"r1","text":"Raise the memory limit from 256 MiB to 384 MiB","evidence":["Peak usage reached 288 MiB"]},{"id":"r2","text":"Warm the cache before shifting traffic","evidence":["Hit ratio recovered after 10 minutes"]}]}',
 d.started_at
FROM deploy_attempts d WHERE d.service_name = 'marketing-site' AND d.status = 'succeeded' ORDER BY d.started_at DESC LIMIT 1;

-- Status page branding and two groups over the seeded components.
UPDATE status_page_settings SET enabled = 1, accent = '#4f8cff', about = 'We post here whenever something is slow or down.', website = 'https://acme.example.com';
INSERT OR IGNORE INTO status_component_groups (id, name, sort_order) VALUES ('g_web', 'Websites', 0), ('g_core', 'Core services', 1);
UPDATE status_components SET group_id = 'g_web' WHERE id IN ('sc_demo_web', 'sc_demo_docs');
UPDATE status_components SET group_id = 'g_core' WHERE id IN ('sc_demo_api', 'sc_demo_worker', 'sc_demo_billing');
