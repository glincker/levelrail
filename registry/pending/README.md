# Pending templates

Working templates that Levelrail cannot deploy yet. The guard refuses the Docker socket
and extra Linux capabilities, so each one waits for a platform feature. They are not
validated or loaded. When the feature lands, move the folder into `registry/templates/`.

| Template | Waiting for |
| --- | --- |
| dockge | not possible under the project profile: Dockge runs compose for its own stacks, which creates containers and networks named after each stack and creates volumes and pulls images, while the profile only allows creates named `<service>-dapi-*` on the app network |
