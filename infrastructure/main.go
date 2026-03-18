package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")

		awsRegion := cfg.Require("awsRegion")
		mongoURI := cfg.RequireSecret("mongoUri")
		jwtSecret := cfg.RequireSecret("jwtSecret")
		sqsQueueURL := cfg.Require("sqsQueueUrl")
		s3Bucket := cfg.Require("s3Bucket")
		cdnDomain := cfg.Get("cdnDomain")
		workerCount := cfg.Get("workerCount")
		apiImageURI := cfg.Require("apiImageUri")
		workerImageURI := cfg.Require("workerImageUri")
		alloyImageURI := cfg.Require("alloyImageUri")
		grafanaAPIKey := cfg.RequireSecret("grafanaApiKey")

		if workerCount == "" {
			workerCount = "5"
		}

		// VPC + ECS cluster + IAM

		vpc, err := createVPC(ctx)
		if err != nil {
			return err
		}

		cluster, err := createECSCluster(ctx)
		if err != nil {
			return err
		}

		roles, err := createIAMRoles(ctx)
		if err != nil {
			return err
		}

		logGroups, err := createLogGroups(ctx)
		if err != nil {
			return err
		}

		// ECS tasks]
		appConfig := &AppConfig{
			AWSRegion:      awsRegion,
			MongoURI:       mongoURI,
			JWTSecret:      jwtSecret,
			SQSQueueURL:    sqsQueueURL,
			S3Bucket:       s3Bucket,
			CDNDomain:      cdnDomain,
			WorkerCount:    workerCount,
			APIImageURI:    apiImageURI,
			WorkerImageURI: workerImageURI,
			AlloyImageURI:  alloyImageURI,
			GrafanaAPIKey:  grafanaAPIKey,
		}

		services, err := createECSServices(ctx, vpc, cluster, roles, logGroups, appConfig)
		if err != nil {
			return err
		}

		// API Gateway
		_, err = createAPIGateway(ctx, services)
		if err != nil {
			return err
		}

		return nil
	})
}
