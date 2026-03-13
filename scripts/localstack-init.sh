#!/bin/sh
# runs automatically when LocalStack is ready
# creates the S3 bucket and SQS queue your app needs
 
echo "initializing LocalStack resources..."
 
awslocal s3 mb s3://image-pipeline-bucket
 
awslocal sqs create-queue --queue-name image-upload-queue
 
echo "LocalStack resources ready"
echo "  S3 bucket : image-pipeline-bucket"
echo "  SQS queue : image-upload-queue"