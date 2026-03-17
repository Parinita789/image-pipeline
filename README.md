# image-pipeline

Production-grade async image processing pipeline built with Go, React, and AWS. Users upload images via presigned S3 URLs — file bytes never touch the API server — and a background worker compresses them and serves via CloudFront CDN.

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

### Frontend
- **React 19** + TypeScript + Vite
- **TanStack Query** — data fetching with auto-polling for processing status
- **Zustand** — auth state (JWT in localStorage)
- **Tailwind CSS** — styling
- **react-dropzone** — drag-and-drop file upload

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
- Node.js 18+
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

### S3 Bucket CORS

The browser uploads directly to S3 via presigned URLs. Your bucket needs a CORS policy:

```bash
aws s3api put-bucket-cors --bucket your-bucket-name --cors-configuration '{
  "CORSRules": [{
    "AllowedOrigins": ["http://localhost:5173"],
    "AllowedMethods": ["PUT", "GET"],
    "AllowedHeaders": ["*"],
    "MaxAgeSeconds": 3600
  }]
}'
```

### Running Locally

```bash
# API server
go run cmd/api/main.go

# Worker (separate terminal)
go run cmd/worker/main.go

# Frontend (separate terminal)
cd frontend && npm install && npm run dev
```

The frontend runs at `http://localhost:5173` and proxies `/api` to the Go server at `:8080`.

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
├── frontend/                    # React SPA
│   ├── src/
│   │   ├── api/                 # Axios client + API functions
│   │   ├── components/          # ImageCard, UploadModal, DeleteConfirmModal, etc.
│   │   ├── hooks/               # useImages, useDeleteImage, useUpload
│   │   ├── pages/               # Login, Register, Dashboard
│   │   ├── store/               # Zustand auth store
│   │   └── types/               # TypeScript interfaces
│   └── vite.config.ts           # Dev proxy to :8080
├── scripts/
│   ├── localstack-init.sh       # Creates S3 bucket (with CORS) + SQS queue
│   └── dev.sh                   # Docker compose rebuild
├── Dockerfile                   # Multi-stage (api + worker targets)
└── docker-compose.yml           # MongoDB, LocalStack, API, Worker
```

## Key Design Decisions

- **Presigned URLs** — file bytes never reach the API server; S3 absorbs all upload bandwidth directly from the browser
- **Idempotency** — safe to retry at every level (prepare, confirm, worker) using per-file idempotency keys
- **Resilience** — all S3, SQS, and MongoDB calls wrapped with retry + exponential backoff
- **Bulk S3 operations** — batch delete uses `DeleteObjects` API (1 call for up to 1000 keys, not N individual calls)
- **Rate limiting** — token bucket per user with auto-cleanup
- **Request-scoped logging** — `request_id` and `user_id` on every log line
- **Graceful shutdown** — API drains in-flight requests, worker drains in-flight jobs before exit

## Testing

```bash
# Unit tests
go test ./internal/services/...

# Integration tests (requires Docker for testcontainers)
go test ./internal/tests/integration/...
```

Integration tests spin up ephemeral MongoDB and LocalStack containers via testcontainers — no external dependencies needed.
