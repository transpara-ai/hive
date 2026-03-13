# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy dependency manifests first for layer caching.
COPY go.mod go.sum ./

# Copy eventgraph dependency (local replace directive in go.mod).
COPY eventgraph/ ./eventgraph/

# Download modules.
RUN go mod download

# Copy source.
COPY . .

# Build the work-server binary.
RUN CGO_ENABLED=0 GOOS=linux go build -o work-server ./cmd/work-server/

# Runtime stage — minimal image, no Go toolchain.
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /build/work-server .

EXPOSE 8080

ENTRYPOINT ["/app/work-server"]
