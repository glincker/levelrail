# Minimal real HTTP-serving fixture for test/e2e/deploy_test.go. Unlike
# internal/deploy/testdata/Dockerfile (deliberately listens on nothing, no
# probe possible), this one actually binds a port and returns a
# distinctive, greppable body, so the full build-to-HTTPS chain has
# something real to prove: a readiness probe that can genuinely pass or
# fail, and an HTTPS response through Caddy that can be checked for exact
# content, not just a 200 status.
#
# busybox's built-in httpd applet, not nginx or a compiled binary: the
# image is a few MB, the build has no compile step, and CI pays for
# neither a slow build nor a slow pull.
FROM busybox:1.36

WORKDIR /srv
RUN echo "hello from levelrail e2e" > index.html

EXPOSE 8080

# -f: stay in the foreground (this is the container's main process, not a
# daemon that would exit immediately and leave nothing running).
# -p: listen on 8080, matching the port test/e2e/deploy_test.go declares
# in its DesiredService and readiness probe.
# -h: serve files out of /srv, where index.html above was written.
CMD ["busybox", "httpd", "-f", "-p", "8080", "-h", "/srv"]
