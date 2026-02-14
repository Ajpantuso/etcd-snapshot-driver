FROM golang:1.20-alpine AS builder

WORKDIR /workspace

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY internal/ internal/

# Build the application
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
	-ldflags="-X github.com/Ajpantuso/etcd-snapshot-driver/pkg/util.Version=${VERSION} \
		-X 'github.com/Ajpantuso/etcd-snapshot-driver/pkg/util.BuildTime=${BUILD_TIME}' \
		-X github.com/Ajpantuso/etcd-snapshot-driver/pkg/util.GitCommit=${COMMIT}" \
	-o /bin/etcd-snapshot-driver ./cmd/etcd-snapshot-driver

# Final stage
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /bin/etcd-snapshot-driver /bin/

USER nonroot:nonroot

ENTRYPOINT ["/bin/etcd-snapshot-driver"]
