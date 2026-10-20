# Build the binary first (scripts/plugins-build.sh does this): CGO_ENABLED=0 GOOS=linux go build -o plugin .
FROM alpine:3.20 AS certs
RUN apk add --no-cache ca-certificates

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY plugin /plugin
ENTRYPOINT ["/plugin"]
