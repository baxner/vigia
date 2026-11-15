# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY core ./core
COPY agent ./agent
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/vigia-agent ./cmd/vigia-agent

FROM alpine:3.20
LABEL org.opencontainers.image.title="vigia-agent" \
      org.opencontainers.image.description="Vigia 24/7 local network monitor agent" \
      org.opencontainers.image.source="https://github.com/baxner/vigia" \
      org.opencontainers.image.licenses="Apache-2.0"
RUN apk add --no-cache ca-certificates iproute2 tzdata && \
    mkdir -p /data && chmod 0700 /data
COPY --from=builder /out/vigia-agent /usr/local/bin/vigia-agent
VOLUME ["/data"]
EXPOSE 7341/tcp 5353/udp
ENTRYPOINT ["/usr/local/bin/vigia-agent"]
CMD ["run", "-data", "/data", "-bind", ":7341"]
