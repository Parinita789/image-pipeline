#!/bin/bash
# Run after any pulumi up or container restart to update API Gateway with new IP
set -e
 
REGION=${AWS_REGION:-us-west-1}
CLUSTER="image-pipeline"
SERVICE="image-pipeline-api"
 
echo "fetching current task IP..."

# get current task ARN
TASK_ARN=$(aws ecs list-tasks \
  --cluster $CLUSTER \
  --service-name $SERVICE \
  --region $REGION \
  --query 'taskArns[0]' \
  --output text)
 
if [ "$TASK_ARN" == "None" ] || [ -z "$TASK_ARN" ]; then
  echo "no running tasks found — is the service running?"
  exit 1
fi

# get network interface ID
ENI_ID=$(aws ecs describe-tasks \
  --cluster $CLUSTER \
  --tasks $TASK_ARN \
  --region $REGION \
  --query 'tasks[0].attachments[0].details[?name==`networkInterfaceId`].value' \
  --output text)

  # get public IP
PUBLIC_IP=$(aws ec2 describe-network-interfaces \
  --network-interface-ids $ENI_ID \
  --region $REGION \
  --query 'NetworkInterfaces[0].Association.PublicIp' \
  --output text)

if [ "$PUBLIC_IP" == "None" ] || [ -z "$PUBLIC_IP" ]; then
  echo "no public IP found — does the task have AssignPublicIp enabled?"
  exit 1
fi 

echo "current public IP: $PUBLIC_IP"

# update pulumi config and redeploy API Gateway integration
cd infrastructure
pulumi config set apiTaskIp $PUBLIC_IP
echo "config updated — run pulumi up to apply"
