ARG VERSION=v0.0.0-dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build-base

ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

RUN apk add --no-cache bash

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

FROM build-base AS client-build

ARG VERSION
ARG COMMIT
ARG BUILD_DATE

RUN VERSION=${VERSION} COMMIT=${COMMIT} BUILD_DATE=${BUILD_DATE} \
        ./scripts/build-client.sh \
    && mkdir -p /out/client/${VERSION} \
    && cp dist/client/${VERSION}/*.tar.gz /out/client/${VERSION}/ \
    && (cd /out/client/${VERSION} && sha256sum *.tar.gz > checksums.txt) \
    && cp dist/client/latest.txt dist/client/latest.json /out/client/

FROM build-base AS server-build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION
ARG COMMIT
ARG BUILD_DATE

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags "-X main.BuildVersion=${VERSION} -X main.BuildCommit=${COMMIT} -X main.BuildDate=${BUILD_DATE} -s -w" \
    -o /out/server ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates su-exec \
    && addgroup -S aist \
    && adduser -S -G aist aist

WORKDIR /var/lib/aist

COPY --from=server-build /out/server /usr/local/bin/server
COPY --from=client-build /out/client /usr/share/aist/client
COPY deploy/server/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod +x /usr/local/bin/docker-entrypoint.sh \
    && mkdir -p /var/lib/aist \
    && chown -R aist:aist /var/lib/aist

ENV AIST_SERVER_ADDR=:8090

EXPOSE 8090
VOLUME ["/var/lib/aist"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8090/v1/healthz >/dev/null 2>&1 || exit 1

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["serve"]
