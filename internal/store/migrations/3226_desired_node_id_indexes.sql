-- Node detail, drain and node resources filter apps and databases by node_id.
CREATE INDEX IF NOT EXISTS idx_desired_services_node_id ON desired_services(node_id);
CREATE INDEX IF NOT EXISTS idx_desired_databases_node_id ON desired_databases(node_id);
