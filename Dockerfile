# The toolchain stages are pinned to the build platform on purpose: image
# references in COPY --from resolve to the *target* platform, so an arm64 build
# would otherwise copy arm64 node/go binaries into an amd64 stage and try to run
# them under QEMU (failing with "Could not open '/lib/ld-musl-aarch64.so.1'").
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24 AS golang
FROM --platform=$BUILDPLATFORM node:26.13.1-alpine3.24 AS node

FROM --platform=$BUILDPLATFORM alpine:3.24 AS base

RUN apk update && \
        apk add --no-cache \
        make \
        gcc \
        git \
        musl-dev \
        libstdc++

COPY --from=golang /usr/local/go/ /usr/local/go/
ENV PATH="/usr/local/go/bin:${PATH}"
ENV CGO_ENABLED=0

COPY --from=node /usr/local/ /usr/local/
ENV NODE_PATH="/usr/local/lib/node_modules"
ENV PATH="/usr/local/bin:${PATH}"

WORKDIR /qingyugist

COPY . .


FROM base AS dev
RUN apk add --no-cache \
    openssl \
    openssh-server \
    curl \
    wget \
    git \
    gnupg \
    xz

EXPOSE 6157 6158 2222 16157

RUN git config --global --add safe.directory /qingyugist
RUN make install

VOLUME /qingyugist

CMD ["make", "watch"]


FROM base AS build

ARG VERSION=dev
ARG TARGETARCH
ENV GOARCH=${TARGETARCH}
RUN make GIT_TAG=v${VERSION}


FROM alpine:3.24 AS prod

ARG VERSION=dev
LABEL org.opencontainers.image.title="QingyuGist" \
      org.opencontainers.image.description="A self-hosted pastebin powered by Git" \
      org.opencontainers.image.source="https://github.com/qingyu-org/Gist" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.licenses="AGPL-3.0"

RUN apk update && \
    apk add --no-cache \
    shadow \
    curl \
    git

RUN addgroup -S qingyugist && \
    adduser -S -G qingyugist -s /bin/ash -g 'QingyuGist User' qingyugist

WORKDIR /app/qingyugist

COPY --from=build --chown=qingyugist:qingyugist /qingyugist/qingyugist .
COPY --from=build --chown=qingyugist:qingyugist /qingyugist/docker ./docker

ENV QYG_HOME=/qingyugist

EXPOSE 6157 6158 2222
VOLUME /qingyugist
HEALTHCHECK --interval=60s --timeout=30s --start-period=15s --retries=3 CMD curl -f http://localhost:6157/healthcheck || exit 1
ENTRYPOINT ["./docker/entrypoint.sh"]
