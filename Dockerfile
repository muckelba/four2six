# Build
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY go.mod main.go ./
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 go build 

# Execute
FROM alpine
ENV WEBHOOK_LISTEN_PORT=8081

WORKDIR /app
COPY --from=build /app/four2six four2six

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -f http://localhost:${WEBHOOK_LISTEN_PORT}/health || exit 1

CMD ["./four2six"]
