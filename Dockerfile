FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/wallet ./cmd/wallet
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/notifier ./cmd/notifier

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/wallet /wallet
COPY --from=builder /bin/notifier /notifier
USER nonroot:nonroot