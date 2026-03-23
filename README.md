# image-pipeline

Production-grade async image processing pipeline built with Go and AWS. Users upload images via presigned S3 URLs — file bytes never touch the API server — and a background worker compresses them and serves via CloudFront CDN.

**Live Demo:** [https://d3vldc1umh6ksf.cloudfront.net](https://d3vldc1umh6ksf.cloudfront.net)

## Architecture

```
Browser                              AWS
  │                                   │
  │  1. POST /api/images/prepare      │
  │ ──────────────► CloudFront ──► API Gateway ──► ECS API
  │ ◄────────────── presigned URLs    │
  │                                   │
  │  2. PUT (file bytes)              │
  │ ──────────────────────────────► S3│
  │                                   │
  │  3. POST /api/images/confirm      │
  │ ──────────────► CloudFront ──► API Gateway ──► ECS API ──► SQS
  │                                   │
  │  4. Static assets (HTML/JS/CSS)   │
  │ ──────────────► CloudFront ──► S3 (frontend bucket)
  │                                   │
  │                            Worker │
  │                              │ poll SQS
  │                              │ download raw from S3
  │                              │ compress (JPEG Q60 / PNG best)
  │                              │ upload compressed to S3
  │                              │ save metadata to MongoDB
  │                              │ serve via CloudFront CDN
```

## Tech Stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.25 |
| Router | Chi v5 |
| Database | MongoDB 7 |
| Queue | AWS SQS |
| Storage | AWS S3 (presigned URLs) |
| CDN | CloudFront |
| Auth | JWT (HS256, 24h expiry) |
| Logging | Zap (structured) + CloudWatch |
| Metrics | Prometheus + Grafana Alloy sidecar |
| Infrastructure | Pulumi (Go), ECS Fargate, API Gateway |
| CI/CD | GitHub Actions |

## Features

- **Presigned S3 uploads** — file bytes never reach the API server; browsers upload directly to S3
- **Async processing** — uploads enqueued to SQS, processed by a scalable worker pool
- **Image compression** — JPEG (Q60) and PNG (best) compression
- **Transformations** — resize, crop, grayscale, sepia, blur, sharpen, invert, watermark, format conversion, background removal
- **Batch operations** — batch transform, batch revert, batch delete (up to 50 per request)
- **Idempotency** — safe request retry via `X-Idempotency-Key` header
- **Rate limiting** — token bucket per user (5 tokens/sec, burst 10)
- **Storage quotas** — per-user storage limits with usage tracking
- **Graceful shutdown** — API drains in-flight requests, worker drains in-flight jobs

## Scalability

The architecture is designed for horizontal scalability — the API and worker are stateless, file bytes never touch the API server, and processing is fully decoupled via SQS.

**What scales today:**
- **API layer** — Go + Chi can handle 1k+ req/s per instance; presigned S3 URLs offload all upload bandwidth from the server
- **SQS** — supports up to 3,000 msg/sec per queue with batching, well beyond current needs
- **S3 + CloudFront** — effectively unlimited for storage and delivery

**What needs tuning for production load:**
- **Worker throughput** — currently 5 workers on a single ECS instance; scale by increasing `WORKER_COUNT` and adding ECS auto-scaling based on SQS queue depth
- **MongoDB** — single instance; would need replica set with read replicas, connection pool tuning.
- **Caching** — no Redis layer; user lookups and storage quota checks hit MongoDB on every request

## API Documentation

Full API documentation is available via **Swagger UI**:

```
http://localhost:8080/swagger/index.html
```

The OpenAPI spec covers all endpoints including auth, image upload/management, transformations, batch operations, and storage. The spec files live in [`docs/`](docs/):

- [`docs/swagger.yaml`](docs/swagger.yaml) — OpenAPI 2.0 spec
- [`docs/swagger.json`](docs/swagger.json) — JSON format

Regenerate after modifying handler annotations:

```bash
swag init -g cmd/api/main.go -o docs
```

## Setup

### Prerequisites

- Go 1.25+
- MongoDB
- AWS account (S3 bucket, SQS queue, CloudFront distribution) or Docker for local dev

### Environment Variables

```env
AWS_REGION=us-west-1
S3_BUCKET=your-bucket-name
SQS_QUEUE_URL=https://sqs.us-west-1.amazonaws.com/123456789/YourQueue
MONGO_URI=mongodb://localhost:27017
MONGO_DB=image_pipeline
JWT_SECRET=your-secret
PORT=8080
WORKER_COUNT=5
CLOUDFRONT_DOMAIN=your-distribution.cloudfront.net
```

### Running Locally

```bash
# API server
go run cmd/api/main.go

# Worker (separate terminal)
go run cmd/worker/main.go
```

### Running with Docker

```bash
# Full stack: MongoDB, LocalStack (S3 + SQS), API, Worker
docker compose up --build
```

LocalStack replaces AWS services locally — S3 bucket and SQS queue are auto-provisioned via `scripts/localstack-init.sh`.

## Upload Flow

```bash
# 1. Prepare — get presigned URLs (no file bytes sent)
curl -X POST http://localhost:8080/images/prepare \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"files": [{"filename": "photo.jpg", "contentType": "image/jpeg", "size": 2048576}]}'

# 2. PUT directly to S3 using the returned presigned URL
curl -X PUT "<presigned-url>" \
  -H "Content-Type: image/jpeg" \
  --data-binary @photo.jpg

# 3. Confirm — tell the API which files landed in S3
curl -X POST http://localhost:8080/images/confirm \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "X-Idempotency-Key: unique-key-123" \
  -d '{"files": [{"key": "raw/userId/reqId_photo.jpg", "filename": "photo.jpg", "requestId": "reqId"}]}'
```

## Project Structure

```
image-pipeline/
├── cmd/
│   ├── api/main.go              # API server entry point
│   └── worker/main.go           # Worker entry point
├── internal/
│   ├── app/                     # App init, routes, graceful shutdown
│   ├── auth/                    # JWT auth handler + service
│   ├── config/                  # Env config loader
│   ├── dto/                     # Data transfer objects
│   ├── handlers/                # HTTP handlers (Swagger-annotated)
│   ├── logger/                  # Zap structured logging
│   ├── metrics/                 # Prometheus metric definitions
│   ├── middleware/              # Rate limiting, request ID, logging, idempotency, metrics
│   ├── models/                  # Domain models
│   ├── queue/                   # SQS publisher + consumer
│   ├── repository/              # MongoDB data access (repository pattern)
│   ├── resilence/               # Retry with exponential backoff + circuit breaker
│   ├── s3/                      # S3 client (stream upload/download, presign, bulk delete)
│   ├── services/                # Business logic
│   ├── tests/                   # Integration tests (testcontainers) + mocks
│   ├── utils/                   # Idempotency hashing, retry helpers
│   └── worker/                  # SQS polling + image processing pool
├── pkg/
│   ├── errors/                  # Centralized AppError types
│   ├── pagination/              # Pagination helpers
│   ├── request/                 # Request parsing
│   ├── response/                # Unified JSON response envelope
│   └── validator/               # Input validation
├── docs/                        # Swagger/OpenAPI spec (auto-generated)
├── infrastructure/              # Pulumi IaC (Go) — full AWS stack
├── monitoring/alloy/            # Grafana Alloy sidecar (Prometheus scrape → Grafana Cloud)
├── scripts/                     # Dev, build, and deploy scripts
├── .github/workflows/           # CI/CD pipelines
├── Dockerfile                   # Multi-stage (api + worker targets)
└── docker-compose.yml           # Local dev stack
```

## Infrastructure (AWS via Pulumi)

The `infrastructure/` directory provisions the full production stack:

| Resource | Details |
|----------|---------|
| **VPC** | Public subnet, internet gateway, security group (ingress 8080) |
| **ECS Fargate** | Cluster with Container Insights |
| **API Service** | 0.25 vCPU / 512 MB, health check on `/health` |
| **Worker Service** | 0.25 vCPU / 512 MB, polls SQS, min-healthy 0% during deploy |
| **Alloy sidecar** | Scrapes `/metrics` → Grafana Cloud |
| **IAM** | Execution role (ECR, CloudWatch, Secrets Manager) + task role (S3, SQS) |
| **Secrets Manager** | MONGO_URI, JWT_SECRET, GRAFANA_API_KEY |
| **CloudWatch** | Log groups with 7-day retention |
| **API Gateway** | HTTP API with `ANY /{proxy+}` → ECS |
| **S3 + CloudFront** | Frontend hosting with OAC, `/api/*` proxied to API Gateway |

### Deploying

```bash
# Infrastructure
cd infrastructure && pulumi up --stack prod

# Backend images
./scripts/push.sh v1

# Frontend
./scripts/deploy-frontend.sh
```

### CI/CD

Push to `main` triggers GitHub Actions: test → build → push to ECR → deploy via Pulumi.

Required secrets: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `PULUMI_ACCESS_TOKEN`.

## Testing

```bash
# Unit tests
go test ./internal/auth/... ./internal/services/...

# Integration tests (requires Docker for testcontainers)
go test ./internal/tests/integration/... -timeout 5m
```

Integration tests spin up ephemeral MongoDB and LocalStack containers via testcontainers — no external dependencies needed.

## Observability

- **Prometheus metrics** at `GET /metrics` — HTTP latency/count, upload pipeline counters, worker job stats, compression ratios
- **Grafana Alloy** sidecar scrapes every 15s, remote-writes to Grafana Cloud
- **Structured logging** via zap — every log line includes `requestId` and `userId`
- **CloudWatch Logs** — container stdout via `awslogs` driver

## Security

- **Presigned URLs** — file bytes never touch the API server
- **Secrets Manager** — sensitive config pulled at runtime, never plaintext env vars
- **CloudFront OAC** — S3 buckets not publicly accessible
- **JWT authentication** — all image endpoints require valid token
- **Rate limiting** — token bucket per user
- **Idempotency** — safe to retry uploads without duplicates
- **CORS** — locked to specific allowed origins

## Key Design Decisions

- **Presigned URLs** — S3 absorbs all upload bandwidth directly from the browser
- **Idempotency** — safe to retry at every level (prepare, confirm, worker) using per-file idempotency keys
- **Resilience** — all S3, SQS, and MongoDB calls wrapped with retry + exponential backoff + circuit breaker
- **Bulk S3 operations** — batch delete uses `DeleteObjects` API (1 call for up to 1000 keys)
- **Request-scoped logging** — `requestId` and `userId` on every log line for traceability
- **Graceful shutdown** — API drains in-flight requests, worker drains in-flight jobs before exit
- **Single CloudFront distribution** — serves both frontend and API, so the frontend uses relative `/api` paths with zero CORS issues
- **SQS over Kafka** — SQS is the right fit at this scale: fully managed, zero ops, ~$0.40/million messages, built-in retry and DLQ support. Kafka adds operational complexity (brokers, partitions, consumer groups) that only pays off at 100k+ msg/sec or when you need ordering guarantees and message replay

## Future Enhancements

- [ ] **Dead letter queue** — isolate poison messages and failed jobs; CloudWatch alarm on DLQ depth for alerting
- [ ] **Cursor-based pagination** — replace `Skip(offset)` with keyset pagination on `_id`/`createdAt` for O(1) page lookups at scale
- [ ] **Redis caching** — cache user profiles and storage quota on the hot read path to reduce MongoDB load
- [ ] **ECS auto-scaling** — target tracking policies: scale API on CPU utilization, scale workers on `ApproximateNumberOfMessagesVisible`
- [ ] **MongoDB replica set** — read replicas for query load, connection pool tuning (`maxPoolSize`)
- [ ] **Pre-generated thumbnails** — generate multiple sizes at upload time to avoid serving full-resolution images in grid views
