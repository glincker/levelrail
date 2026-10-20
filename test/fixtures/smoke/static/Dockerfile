FROM busybox:1.36
WORKDIR /srv
COPY MARKER healthz /srv/
RUN cp MARKER index.html
USER 65534:65534
EXPOSE 8080
CMD ["busybox", "httpd", "-f", "-p", "8080", "-h", "/srv"]
