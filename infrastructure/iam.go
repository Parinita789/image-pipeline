package main

import (
	"encoding/json"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type IAMRoles struct {
	TaskExecutionRole *iam.Role
	TaskRole          *iam.Role
}

func createIAMRoles(ctx *pulumi.Context) (*IAMRoles, error) {
	assumeRolePolicy, _ := json.Marshal(map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect":    "Allow",
				"Principal": map[string]interface{}{"Service": "ecs-tasks.amazonaws.com"},
				"Action":    "sts:AssumeRole",
			},
		},
	})

	// Task Execution Role - pull ECR images, write CloudWatch logs
	executionRole, err := iam.NewRole(ctx, "image-pipeline-execution-role", &iam.RoleArgs{
		Name:             pulumi.String("image-pipeline-execution-role"),
		AssumeRolePolicy: pulumi.String(string(assumeRolePolicy)),
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-execution-role"),
		},
	})
	if err != nil {
		return nil, err
	}

	// attach AWS managed policy — covers ECR pull + CloudWatch logs
	_, err = iam.NewRolePolicyAttachment(ctx, "execution-role-policy", &iam.RolePolicyAttachmentArgs{
		Role:      executionRole.Name,
		PolicyArn: pulumi.String("arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"),
	})
	if err != nil {
		return nil, err
	}

	// Task Role - app runs as this role — access S3, SQS, CloudWatch metrics
	taskRole, err := iam.NewRole(ctx, "image-pipeline-task-role", &iam.RoleArgs{
		Name:             pulumi.String("image-pipeline-task-role"),
		AssumeRolePolicy: pulumi.String(string(assumeRolePolicy)),
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-task-role"),
		},
	})
	if err != nil {
		return nil, err
	}

	// inline policy — S3, SQS, CloudWatch
	taskPolicy, _ := json.Marshal(map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect": "Allow",
				"Action": []string{
					"s3:PutObject",
					"s3:GetObject",
					"s3:DeleteObject",
					"s3:CopyObject",
					"s3:ListBucket",
				},
				"Resource": []string{
					"arn:aws:s3:::*",
					"arn:aws:s3:::*/*",
				},
			},
			{
				"Effect": "Allow",
				"Action": []string{
					"sqs:SendMessage",
					"sqs:ReceiveMessage",
					"sqs:DeleteMessage",
					"sqs:GetQueueAttributes",
				},
				"Resource": "*",
			},
			{
				"Effect": "Allow",
				"Action": []string{
					"cloudwatch:PutMetricData",
					"logs:CreateLogStream",
					"logs:PutLogEvents",
					"logs:DescribeLogStreams",
				},
				"Resource": "*",
			},
		},
	})

	_, err = iam.NewRolePolicy(ctx, "image-pipeline-task-policy", &iam.RolePolicyArgs{
		Name:   pulumi.String("image-pipeline-task-policy"),
		Role:   taskRole.Name,
		Policy: pulumi.String(string(taskPolicy)),
	})
	if err != nil {
		return nil, err
	}

	ctx.Export("executionRoleArn", executionRole.Arn)
	ctx.Export("taskRoleArn", taskRole.Arn)

	return &IAMRoles{
		TaskExecutionRole: executionRole,
		TaskRole:          taskRole,
	}, nil
}
