# Build stage
FROM golang:1.25-alpine3.23 AS builder
WORKDIR /app
COPY . .
RUN go build -o main main.go

# Run stage
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/main .
RUN mkdir -p /app/config
COPY config/app.env /app/config/app.env
COPY scripts/start.sh .
COPY scripts/wait-for.sh .
COPY db/migration ./db/migration

EXPOSE 50051
EXPOSE 8080
ENTRYPOINT [ "/app/start.sh" ]