# One of two sibling fixtures (see ../worker/Dockerfile) used by
# test/e2e/multi_service_test.go: proves build.BaseDirectory scopes each
# fanned-out service to its own subdirectory of one shared checkout,
# the way a real monorepo app.yaml would.
FROM busybox:1.36

WORKDIR /srv
RUN echo "hello from levelrail e2e multi web" > index.html

EXPOSE 8080

CMD ["busybox", "httpd", "-f", "-p", "8080", "-h", "/srv"]
