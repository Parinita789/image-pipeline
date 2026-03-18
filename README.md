# image-pipeline

Production-grade async image processing pipeline built with Go and AWS. Users upload images via presigned S3 URLs — file bytes never touch the API server — and a background worker compresses them and serves via CloudFront CDN.

## Architecture

```
Browser                          AWS
  │                               │
  │  1. POST /images/prepare      │
  │ ─────────────────────► API    │
  │ ◄───── presigned URLs         │
  │                               │
  │  2. PUT (file bytes)          │
  │ ──────────────────────────► S3│
  │                               │
  │  3. POST /images/confirm      │
  │ ─────────────────────► API ──► SQS
  │                               │
  │                        Worker │
  │                          │ poll SQS
  │                          │ download raw from S3
  │                          │ compress (JPEG Q60 / PNG best)
  │                          │ upload compressed to S3
  │                          │ save metadata to MongoDB
  │                          │ serve via CloudFront CDN
```

## Stack

### Backend
- **Go** — chi router, zap structured logging
- **AWS** — S3 (storage + presigned URLs), SQS (job queue), CloudFront (CDN)
- **MongoDB** — metadata, idempotency tracking, user accounts

## API Endpoints

### Public
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/auth/register` | Register a new user |
| POST | `/auth/login` | Login, returns JWT |

### Protected (JWT required)
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/images/prepare` | Get presigned S3 PUT URLs for files |
| POST | `/images/confirm` | Confirm upload, enqueue processing (requires `X-Idempotency-Key`) |
| GET | `/images` | List images (paginated, filterable by search/status) |
| GET | `/images/{requestId}` | Get single image by request ID |
| DELETE | `/image/{id}` | Delete a single image |
| DELETE | `/images` | Batch delete up to 50 images |

## Setup

### Prerequisites
- Go 1.25+
- MongoDB
- AWS account (S3 bucket, SQS queue, CloudFront distribution)

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
# Full stack (MongoDB, LocalStack, API, Worker)
docker compose up --build
```

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

## Batch Delete

```bash
curl -X DELETE http://localhost:8080/images \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"ids": ["id1", "id2", "id3"]}'
```

Returns which IDs were deleted and which S3 keys failed cleanup:
```json
{"data": {"deleted": ["id1", "id2", "id3"], "failed": []}}
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
│   ├── handlers/                # HTTP handlers
│   ├── logger/                  # Zap structured logging
│   ├── middleware/              # Rate limiting, request ID, logging, idempotency
│   ├── models/                  # Domain models
│   ├── queue/                   # SQS publisher + consumer
│   ├── repository/              # MongoDB data access (repository pattern)
│   ├── resilence/               # Retry with exponential backoff
│   ├── s3/                      # S3 client (stream upload/download, presign, bulk delete)
│   ├── services/                # Business logic
│   ├── tests/integration/       # Integration tests (testcontainers)
│   └── worker/                  # SQS polling + image processing pool
├── pkg/
│   ├── response/                # Unified JSON response envelope
│   ├── request/                 # Request parsing
│   ├── validator/               # Input validation
│   ├── pagination/              # Pagination helpers
│   └── errors/                  # Custom error types
├── infrastructure/                 # Pulumi IaC (Go)
│   ├── main.go                    # Entrypoint — config, orchestration
│   ├── vpc.go                     # VPC, subnet, IGW, security group
│   ├── ecs.go                     # ECS cluster
│   ├── tasks.go                   # Task definitions + services (API, Worker, Alloy sidecar)
│   ├── iam.go                     # Execution role + task role
│   ├── cloudwatch.go              # Log groups
│   ├── api_gateway.go             # HTTP API Gateway → ECS
│   └── Pulumi.prod.yaml           # Stack config (secrets encrypted)
├── monitoring/
│   └── alloy/
│       ├── config.river           # Prometheus scrape + remote write to Grafana Cloud
│       └── Dockerfile
├── scripts/
│   ├── localstack-init.sh         # Creates S3 bucket (with CORS) + SQS queue
│   ├── dev.sh                     # Docker compose rebuild
│   └── push.sh                    # Build & push API/Worker images to ECR
├── Dockerfile                     # Multi-stage (api + worker targets)
└── docker-compose.yml             # MongoDB, LocalStack, API, Worker
```

## Infrastructure (AWS via Pulumi)

The `infrastructure/` directory contains Pulumi IaC (Go) that provisions the full production stack on AWS:

| Resource | Details |
|----------|---------|
| **VPC** | Public subnet, internet gateway, security group (ingress 8080, 4317, 4318) |
| **ECS Fargate** | Cluster with Container Insights enabled |
| **API Service** | 0.25 vCPU / 512 MB, health check on `/health`, public IP |
| **Worker Service** | 0.25 vCPU / 512 MB, polls SQS, min-healthy 0% during deploy |
| **Grafana Alloy sidecar** | Runs alongside API container, scrapes `/metrics` → Grafana Cloud |
| **IAM** | Task execution role (ECR pull, CloudWatch logs) + task role (S3, SQS full access) |
| **CloudWatch** | Log groups `/image-pipeline/api` and `/image-pipeline/worker` (7-day retention) |
| **API Gateway** | HTTP API with catch-all `ANY /{proxy+}` → ECS API task, auto-deploy stage |

### Deploying

```bash
cd infrastructure
pulumi up --stack prod
```

Config values are set via `Pulumi.prod.yaml` — secrets (mongoUri, jwtSecret, grafanaApiKey) are encrypted by Pulumi.

### Building & Pushing Images

```bash
# Build and push API + Worker images to ECR
./scripts/push.sh v1
```

## Observability

- **Prometheus metrics** exposed at `GET /metrics` — HTTP request duration/count, upload pipeline counters, worker job stats, auth operations, compression ratios
- **Grafana Alloy** sidecar scrapes the API every 15s and remote-writes to Grafana Cloud Hosted Prometheus
- **Structured logging** via zap — every log line includes `requestId` and `userId`
- **CloudWatch Logs** — all container stdout forwarded via `awslogs` driver

Alloy config: `monitoring/alloy/config.river`

## Error Handling

All errors use a centralized `AppError` system (`pkg/errors/`):

```go
// Machine-readable codes, consistent HTTP status mapping
var ErrImageNotFound  = New(404, "IMAGE_NOT_FOUND", "image not found")
var ErrImageForbidden = New(403, "IMAGE_FORBIDDEN", "you do not own this image")
```

Response envelope is always `{ status, code, message, data }` — clients can switch on `code` for programmatic error handling.

## Key Design Decisions

- **Presigned URLs** — file bytes never reach the API server; S3 absorbs all upload bandwidth directly from the browser
- **Idempotency** — safe to retry at every level (prepare, confirm, worker) using per-file idempotency keys
- **Resilience** — all S3, SQS, and MongoDB calls wrapped with retry + exponential backoff
- **Bulk S3 operations** — batch delete uses `DeleteObjects` API (1 call for up to 1000 keys, not N individual calls)
- **Rate limiting** — token bucket per user with auto-cleanup
- **Request-scoped logging** — `requestId` and `userId` on every log line
- **Graceful shutdown** — API drains in-flight requests, worker drains in-flight jobs before exit
- **Centralized errors** — typed `AppError` constants with HTTP code, machine-readable code, and message; single source of truth across handlers, middleware, and services

## Testing

```bash
# Unit tests
go test ./internal/services/...

# Integration tests (requires Docker for testcontainers)
go test ./internal/tests/integration/...
```

Integration tests spin up ephemeral MongoDB and LocalStack containers via testcontainers — no external dependencies needed.
