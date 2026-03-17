#!/bin/sh
# runs automatically when LocalStack is ready
# creates the S3 bucket and SQS queue your app needs
 
echo "initializing LocalStack resources..."
 
awslocal s3 mb s3://image-pipeline-bucket

awslocal s3api put-bucket-cors --bucket image-pipeline-bucket --cors-configuration '{
  "CORSRules": [{
    "AllowedOrigins": ["http://localhost:5173"],
    "AllowedMethods": ["PUT", "GET"],
    "AllowedHeaders": ["*"],
    "MaxAgeSeconds": 3600
  }]
}'

awslocal sqs create-queue --queue-name image-upload-queue

echo "LocalStack resources ready"
echo "  S3 bucket : image-pipeline-bucket (CORS configured)"
echo "  SQS queue : image-upload-queue"