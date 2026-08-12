# Minimal fixture for the Phase 0 BuildKit spike (internal/build). Nothing
# fancy on purpose: one base image, one RUN, one COPY, enough to prove the
# dockerfile.v0 frontend and the local image store round trip actually
# work end to end.
FROM alpine:3.20

RUN echo "levelrail buildkit spike" > /etc/levelrail-spike

COPY hello.txt /hello.txt

CMD ["cat", "/hello.txt"]
