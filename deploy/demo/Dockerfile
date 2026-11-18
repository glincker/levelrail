# Read-only hosted demo as a container: the released binary plus a shell for the reseed entrypoint.
# Build context is deploy/demo. Bump LEVELRAIL_VERSION to roll the demo to a new release.
ARG LEVELRAIL_VERSION=v1.5.1
FROM ghcr.io/glincker/levelrail:${LEVELRAIL_VERSION} AS bin

FROM alpine:3.20
RUN adduser -D -u 10001 demo && mkdir -p /data && chown demo /data
COPY --from=bin /app/levelrail /usr/local/bin/levelrail
COPY --from=bin /app/brand.yaml /app/brand.yaml
COPY container-entrypoint.sh /usr/local/bin/container-entrypoint.sh
USER demo
WORKDIR /app
ENV APP_DEMO_MODE=1 APP_DEMO_CONTAINER=1 APP_HTTP_ADDR=0.0.0.0:8080 APP_DEMO_RESET_INTERVAL=30m
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/container-entrypoint.sh"]
