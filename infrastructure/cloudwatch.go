package main

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudwatch"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type LogGroups struct {
	API    *cloudwatch.LogGroup
	Worker *cloudwatch.LogGroup
}

func createLogGroups(ctx *pulumi.Context) (*LogGroups, error) {
	apiLogs, err := cloudwatch.NewLogGroup(ctx, "api-logs", &cloudwatch.LogGroupArgs{
		Name:            pulumi.String("/image-pipeline/api"),
		RetentionInDays: pulumi.Int(30),
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-api-logs"),
		},
	})
	if err != nil {
		return nil, err
	}

	// worker log group
	workerLogs, err := cloudwatch.NewLogGroup(ctx, "worker-logs", &cloudwatch.LogGroupArgs{
		Name:            pulumi.String("/image-pipeline/worker"),
		RetentionInDays: pulumi.Int(30),
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-worker-logs"),
		},
	})
	if err != nil {
		return nil, err
	}

	ctx.Export("apiLogGroup", apiLogs.Name)
	ctx.Export("workerLogGroup", workerLogs.Name)

	return &LogGroups{
		API:    apiLogs,
		Worker: workerLogs,
	}, nil
}
