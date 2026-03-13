FROM golang:1.25-alpine As builder

WORKDIR /app

# download deps
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# build both binaries
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/api     ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/worker  ./cmd/worker

# API image
FROM alpine:3.19 AS api
RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /bin/api /api
EXPOSE 8080
ENTRYPOINT [ "/api" ]

# Worker image
FROM alpine:3.19 AS worker
RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /bin/worker /worker
ENTRYPOINT [ "/worker" ]