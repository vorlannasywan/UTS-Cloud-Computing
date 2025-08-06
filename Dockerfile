# Stage 1: Build
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /healthapp-backend

# Stage 2: Run
FROM alpine:latest

WORKDIR /app
COPY --from=builder /healthapp-backend /app/healthapp-backend

ENV PORT=8080

EXPOSE 8080

CMD ["/app/healthapp-backend"]
