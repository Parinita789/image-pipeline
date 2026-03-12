# image-pipeline

Async image processing pipeline built with Go and AWS. Upload an image, get back raw and compressed versions stored in S3.

## How it works

```
POST /image/upload
  → stream to S3 (tmp)
  → publish to SQS
  → return 202

Worker
  → copy tmp → raw
  → compress
  → upload compressed
  → save metadata
```

## Stack

- **Go** — chi, uber/zap, gobreaker
- **AWS** — S3, SQS
- **MongoDB**

## Setup

```env
MONGO_URI=mongodb://localhost:27017/image-pipeline
AWS_REGION=us-east-1
S3_BUCKET=your-bucket
SQS_QUEUE_URL=your-queue-url
JWT_SECRET=your-secret
```

```bash
# API
go run cmd/api/main.go

# Worker (separate terminal)
go run cmd/worker/main.go
```

## Upload

```bash
curl -X POST http://localhost:8080/image/upload \
  -H "Authorization: Bearer <token>" \
  -H "X-Idempotency-Key: unique-key" \
  -H "X-File-Name: photo.jpg" \
  -F "file=@photo.jpg"
```

```json
{ "status": "processing", "request_id": "0efewfgb123" }
```

## Key Design Decisions

- **Streaming** — images pipe directly from request to S3, constant RAM usage
- **Idempotency** — safe to retry at API, queue, and worker level
- **Circuit breaker + retry** — all S3, SQS, and DB calls are wrapped
- **Per-user rate limiting** — token bucket, auto-cleanup
- **Request-scoped logs** — `request_id` and `user_id` on every log line