# Multi-stage build for multi-arch support
# Build stage
FROM --platform=$BUILDPLATFORM golang:1.25 as builder

ARG TARGETPLATFORM

WORKDIR /app
COPY . .

RUN BUILD_PLATFORMS=$TARGETPLATFORM make build

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /app/bin/$TARGETPLATFORM/ /

