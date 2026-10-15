# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.24-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
    && mkdir -p /out/data/uploads

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/api /app/api
# The uploads volume must be writable by the nonroot uid 65532.
COPY --from=build --chown=65532:65532 /out/data /data
ENV PORT=8080 \
    UPLOAD_DIR=/data/uploads
EXPOSE 8080
VOLUME ["/data/uploads"]
USER 65532:65532
ENTRYPOINT ["/app/api"]
