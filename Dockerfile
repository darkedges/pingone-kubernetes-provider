# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

ARG VERSION=dev
ARG GIT_COMMIT=none
ARG BUILD_DATE=unknown

COPY api/       api/
COPY controllers/ controllers/
COPY internal/  internal/
COPY main.go    .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags "-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${GIT_COMMIT} \
      -X main.buildDate=${BUILD_DATE}" \
    -o manager ./main.go

# Runtime stage
# Alpine runtime keeps the image small while providing `update-ca-certificates`
# so custom trust bundles can be refreshed when needed.
FROM alpine:3.20

RUN apk add --no-cache ca-certificates && update-ca-certificates

WORKDIR /
COPY --from=builder /workspace/manager /manager

USER 65532:65532

ENTRYPOINT ["/manager"]
