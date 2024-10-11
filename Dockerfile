# Build
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY go.mod main.go ./
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 go build 

# Execute
FROM alpine

WORKDIR /app
COPY --from=build /app/four2six four2six

CMD ["./four2six"]
