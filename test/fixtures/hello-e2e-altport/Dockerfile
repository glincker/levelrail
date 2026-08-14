# Near-copy of test/fixtures/hello-e2e/Dockerfile, used by
# test/e2e/port_mapping_test.go instead of test/e2e/deploy_test.go. Every
# other fixture in this directory listens on 8080, which means nothing in
# the e2e suite actually proves the app spec's port field is honored
# end-to-end rather than silently ignored or hardcoded somewhere in the
# pipeline. This fixture listens on 9090, a distinctly different port, and
# returns its own distinctive body so a passing test can only mean traffic
# genuinely reached this specific container on this specific port.
#
# busybox's built-in httpd applet, same reasoning as hello-e2e/Dockerfile:
# a few MB image, no compile step, cheap to build and pull in CI.
FROM busybox:1.36

WORKDIR /srv
RUN echo "hello from levelrail e2e alt port" > index.html

EXPOSE 9090

# -f: stay in the foreground (this is the container's main process, not a
# daemon that would exit immediately and leave nothing running).
# -p: listen on 9090, matching the port test/e2e/port_mapping_test.go
# declares in its DesiredService and readiness probe. Deliberately not
# 8080, the port every other fixture in this repo happens to use.
# -h: serve files out of /srv, where index.html above was written.
CMD ["busybox", "httpd", "-f", "-p", "9090", "-h", "/srv"]
