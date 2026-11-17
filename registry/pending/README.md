# Pending templates

Working templates that Levelrail cannot deploy yet. The guard refuses the Docker socket
and extra Linux capabilities, so each one waits for a platform feature. They are not
validated or loaded. When the feature lands, move the folder into `registry/templates/`.

| Template | Waiting for |
| --- | --- |
| dozzle | read-only, scoped Docker API access through a socket proxy |
| dockge | scoped Docker API access (compose control) through a socket proxy |
| wg-easy | an approved capability grant for NET_ADMIN, NET_RAW and SYS_MODULE |
