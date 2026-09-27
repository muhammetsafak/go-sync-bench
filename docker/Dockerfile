# Build context: the repository root.
#   docker build --network none -f docker/Dockerfile -t go-sync-bench .
# The harness has no dependencies outside the standard library, so the build
# needs no network once the two base images are present locally.
FROM golang:1.27.1-trixie AS build
ENV GOTOOLCHAIN=local CGO_ENABLED=0
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -trimpath -o /out/syncbench ./cmd/syncbench

FROM debian:trixie-slim
COPY --from=build /out/syncbench /app/syncbench
COPY bench/matrix.json /app/matrix.json
USER 65534:65534
WORKDIR /tmp
ENTRYPOINT ["/app/syncbench"]
