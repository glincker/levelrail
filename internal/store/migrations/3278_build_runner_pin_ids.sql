-- Pins resolve by node ID only, so rewrite pins that stored a node name.
UPDATE app_build_runner
SET node_id = (SELECT n.id FROM nodes n WHERE n.name = app_build_runner.node_id)
WHERE node_id != ''
  AND NOT EXISTS (SELECT 1 FROM nodes n WHERE n.id = app_build_runner.node_id)
  AND EXISTS (SELECT 1 FROM nodes n WHERE n.name = app_build_runner.node_id);
