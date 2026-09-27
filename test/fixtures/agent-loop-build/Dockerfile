# Build-error fixture for test/e2e/agent_loop_test.go: requirements.txt pins a
# version ./index does not carry, so install-deps.sh fails the build
# offline and deterministically.
FROM busybox:1.36
WORKDIR /srv
COPY requirements.txt install-deps.sh ./
COPY index ./index
RUN sh install-deps.sh
EXPOSE 8080
CMD ["busybox", "httpd", "-f", "-p", "8080", "-h", "/srv/www"]
