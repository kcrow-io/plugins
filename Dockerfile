# Multi-stage build for multi-arch support
# Build stage
FROM --platform=$BUILDPLATFORM golang:1.24 as builder

ARG TARGETPLATFORM

WORKDIR /app
COPY . .

RUN BUILD_PLATFORMS=$TARGETPLATFORM make build

# Runtime image
FROM python:3.11-slim

ARG TARGETPLATFORM

ARG GIT_COMMIT_VERSION
ENV GIT_COMMIT_VERSION=${GIT_COMMIT_VERSION}
ARG GIT_COMMIT_TIME
ENV GIT_COMMIT_TIME=${GIT_COMMIT_TIME}
ARG VERSION
ENV VERSION=${VERSION}

WORKDIR /
RUN mkdir -p /opt/kcrow/bin
COPY --from=builder /app/bin/$TARGETPLATFORM/ /opt/kcrow/bin/
COPY install/install_nri_plugins.py .

# No need to install dependencies as they're handled by install script
CMD ["python3", "install_nri_plugins.py"]
