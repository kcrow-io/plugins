# Multi-stage build for multi-arch support
# Build stage
FROM --platform=$BUILDPLATFORM golang:1.26.3-bookworm as builder

ARG TARGETPLATFORM

WORKDIR /app
COPY . .

ENV GOPROXY=https://goproxy.cn,direct

RUN BUILD_PLATFORMS=$TARGETPLATFORM make build

FROM busybox:1.36.1

ARG TARGETPLATFORM

COPY --from=builder /app/bin/$TARGETPLATFORM/ /

