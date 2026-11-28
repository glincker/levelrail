-- Node id per replica index when an app's replicas are spread over nodes; empty means all replicas run on node_id.
ALTER TABLE desired_services ADD COLUMN replica_nodes TEXT NOT NULL DEFAULT '[]';
