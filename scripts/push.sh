#!/bin/bash
set -e

# set account ID and region
VERSION=${1:?usage: ./scripts/push.sh VERSION}
AWS_REGION=${AWS_REGION:-us-west-1}  
export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
export ECR_BASE=$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com

echo "VERSION=$VERSION"
echo "AWS_REGION=$AWS_REGION"
echo "ECR=$ECR_BASE"

# authenticate
aws ecr get-login-password --region $AWS_REGION | docker login --username AWS --password-stdin $ECR_BASE

# build and push images
echo "building and images..."
docker buildx build --platform linux/amd64 --target api --push \
  -t $ECR_BASE/image-pipeline/api:$VERSION \
  -t $ECR_BASE/image-pipeline/api:latest .

docker buildx build --platform linux/amd64 --target worker --push \
  -t $ECR_BASE/image-pipeline/worker:$VERSION \
  -t $ECR_BASE/image-pipeline/worker:latest .

docker buildx build --platform linux/amd64 --push \
  -t $ECR_BASE/image-pipeline/alloy:latest \
  monitoring/alloy/

# # push API
# echo "pushing images..."
# docker push $ECR_BASE/image-pipeline/api:$VERSION
# docker push $ECR_BASE/image-pipeline/api:latest
# docker push $ECR_BASE/image-pipeline/worker:$VERSION
# docker push $ECR_BASE/image-pipeline/worker:latest 

# update pulumi config with new image URIs
echo "updating pulumi config..."
cd infrastructure
pulumi config set apiImageUri $ECR_BASE/image-pipeline/api:$VERSION
pulumi config set workerImageUri $ECR_BASE/image-pipeline/worker:$VERSION

# get current task IP and update config
cd ..
./scripts/update-api-gateway.sh

echo "deploying..."
cd infrastructure
pulumi up --yes

echo ""
echo "deploy complete!"
echo "  api:    $ECR_BASE/image-pipeline/api:$VERSION"
echo "  worker: $ECR_BASE/image-pipeline/worker:$VERSION"
echo "  url:    $(pulumi stack output apiGatewayUrl 2>/dev/null || echo 'run pulumi stack output apiGatewayUrl')"
