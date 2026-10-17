FROM golang:1.26-alpine AS build
WORKDIR /src
COPY main.go .
RUN go mod init bench-app && CGO_ENABLED=0 go build -o /bench-app .

FROM scratch
COPY --from=build /bench-app /bench-app
EXPOSE 8080
ENTRYPOINT ["/bench-app"]
