# Build the binary first: CGO_ENABLED=0 GOOS=linux go build -o plugin ./test/fixtures/plugin-notify
FROM scratch
COPY plugin /plugin
ENTRYPOINT ["/plugin"]
