# Fixture for test/e2e/loadbalancer_test.go. Unlike
# test/fixtures/hello-e2e (a static body baked in at build time, so every
# replica built from it is byte-identical), this one generates its
# response body at container *start* time from its own hostname, which
# Docker defaults to the container's own short ID. Every replica this
# test's reconciler creates is a separate container with a separate
# hostname, so this is what lets the test tell which real container
# answered a given real HTTP request, the thing a shared static body
# could never prove.
FROM busybox:1.36

WORKDIR /srv
EXPOSE 8080

CMD sh -c "echo hello from levelrail e2e lb replica $(hostname) > /srv/index.html && busybox httpd -f -p 8080 -h /srv"
