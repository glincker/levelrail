# See ../web/Dockerfile: this sibling fixture listens on a distinctly
# different port (9091) and returns its own distinctive body, so a
# passing test can only mean traffic reached this specific fanned-out
# service, not the other one.
FROM busybox:1.36

WORKDIR /srv
RUN echo "hello from levelrail e2e multi worker" > index.html

EXPOSE 9091

CMD ["busybox", "httpd", "-f", "-p", "9091", "-h", "/srv"]
