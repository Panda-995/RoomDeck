FROM node:22.22.2-alpine AS frontend
WORKDIR /build/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27.1-alpine AS backend
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH:-$(go env GOARCH)} go build -trimpath -ldflags="-s -w" -o /roomdeck ./cmd/roomdeck

FROM alpine:3.23.3
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 10001 roomdeck \
    && adduser -D -H -u 10001 -G roomdeck roomdeck \
    && mkdir -p /data /app/web \
    && chown 10001:10001 /data
COPY --from=backend /roomdeck /app/roomdeck
COPY --from=frontend /build/web/dist /app/web
COPY deploy/init-media.sh /app/init-media.sh
LABEL org.opencontainers.image.source="https://github.com/Panda-995/RoomDeck"
WORKDIR /app
ENV DATA_DIR=/data WEB_DIR=/app/web LISTEN_ADDR=:8080
USER 10001:10001
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 CMD ["/app/roomdeck", "-healthcheck"]
ENTRYPOINT ["/app/roomdeck"]
