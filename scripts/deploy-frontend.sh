#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
FRONTEND_ROOT="$(dirname "$PROJECT_ROOT")/image-pipeline-frontend"

# Build frontend
echo "building frontend..."
cd "$FRONTEND_ROOT"
npm ci
npm run build

# Read Pulumi outputs
cd "$PROJECT_ROOT/infrastructure"
BUCKET_NAME=$(pulumi stack output frontendBucketName)
DISTRIBUTION_ID=$(pulumi stack output distributionId)

echo "bucket:       $BUCKET_NAME"
echo "distribution: $DISTRIBUTION_ID"

# Sync hashed assets with long cache
echo "uploading assets..."
aws s3 sync "$FRONTEND_ROOT/dist/" "s3://$BUCKET_NAME" \
  --exclude "index.html" \
  --cache-control "public, max-age=31536000, immutable" \
  --delete

# Upload index.html with no-cache
echo "uploading index.html..."
aws s3 cp "$FRONTEND_ROOT/dist/index.html" "s3://$BUCKET_NAME/index.html" \
  --cache-control "no-cache, no-store, must-revalidate"

# Invalidate CloudFront cache
echo "invalidating CloudFront cache..."
aws cloudfront create-invalidation \
  --distribution-id "$DISTRIBUTION_ID" \
  --paths "/*" \
  --no-cli-pager

echo ""
echo "deploy complete!"
echo "  url: $(pulumi stack output frontendUrl)"
