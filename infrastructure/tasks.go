package main

import (
	"encoding/json"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type AppConfig struct {
	AWSRegion      string
	MongoURI       pulumi.StringOutput
	JWTSecret      pulumi.StringOutput
	SQSQueueURL    string
	S3Bucket       string
	CDNDomain      string
	WorkerCount    string
	APIImageURI    string
	WorkerImageURI string
	AlloyImageURI  string
	GrafanaAPIKey  pulumi.StringOutput
}

type ECSServices struct {
	APIService    *ecs.Service
	WorkerService *ecs.Service
}

func createECSServices(
	ctx *pulumi.Context,
	vpc *VPCResources,
	cluster *ECSCluster,
	roles *IAMRoles,
	logs *LogGroups,
	cfg *AppConfig,
) (*ECSServices, error) {
	apiTaskDef, err := createAPITaskDef(ctx, roles, logs, cfg)
	if err != nil {
		return nil, err
	}

	apiService, err := ecs.NewService(ctx, "api-service", &ecs.ServiceArgs{
		Name:           pulumi.String("image-pipeline-api"),
		Cluster:        cluster.Cluster.Arn,
		TaskDefinition: apiTaskDef.Arn,
		DesiredCount:   pulumi.Int(1),
		LaunchType:     pulumi.String("FARGATE"),
		NetworkConfiguration: &ecs.ServiceNetworkConfigurationArgs{
			Subnets:        pulumi.StringArray{vpc.PublicSubnet.ID()},
			SecurityGroups: pulumi.StringArray{vpc.SecurityGroup.ID()},
			AssignPublicIp: pulumi.Bool(true),
		},
		DeploymentMinimumHealthyPercent: pulumi.Int(100),
		DeploymentMaximumPercent:        pulumi.Int(200),
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-api"),
		},
	})
	if err != nil {
		return nil, err
	}

	workerTaskDef, err := createWorkerTaskDef(ctx, roles, logs, cfg)
	if err != nil {
		return nil, err
	}

	workerService, err := ecs.NewService(ctx, "worker-service", &ecs.ServiceArgs{
		Name:           pulumi.String("image-pipeline-worker"),
		Cluster:        cluster.Cluster.Arn,
		TaskDefinition: workerTaskDef.Arn,
		DesiredCount:   pulumi.Int(1),
		LaunchType:     pulumi.String("FARGATE"),
		NetworkConfiguration: &ecs.ServiceNetworkConfigurationArgs{
			Subnets:        pulumi.StringArray{vpc.PublicSubnet.ID()},
			SecurityGroups: pulumi.StringArray{vpc.SecurityGroup.ID()},
			AssignPublicIp: pulumi.Bool(true), // needs public IP to reach SQS, S3, MongoDB Atlas
		},
		DeploymentMinimumHealthyPercent: pulumi.Int(0), // worker can go to 0 during deploy
		DeploymentMaximumPercent:        pulumi.Int(200),
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-worker"),
		},
	})
	if err != nil {
		return nil, err
	}

	ctx.Export("apiServiceName", apiService.Name)
	ctx.Export("workerServiceName", workerService.Name)

	return &ECSServices{
		APIService:    apiService,
		WorkerService: workerService,
	}, nil
}

func createAPITaskDef(
	ctx *pulumi.Context,
	roles *IAMRoles,
	logs *LogGroups,
	cfg *AppConfig,
) (*ecs.TaskDefinition, error) {
	containerDef := pulumi.All(cfg.MongoURI, cfg.JWTSecret, cfg.GrafanaAPIKey).ApplyT(
		func(args []interface{}) (string, error) {
			mongoURI := args[0].(string)
			jwtSecret := args[1].(string)
			grafanaAPIKey := args[2].(string)

			def := []map[string]interface{}{
				{
					"name":  "api",
					"image": cfg.APIImageURI,
					"portMappings": []map[string]interface{}{
						{"containerPort": 8080, "protocol": "tcp"},
					},
					"environment": []map[string]string{
						{"name": "AWS_REGION", "value": cfg.AWSRegion},
						{"name": "S3_BUCKET", "value": cfg.S3Bucket},
						{"name": "SQS_QUEUE_URL", "value": cfg.SQSQueueURL},
						{"name": "CDN_DOMAIN", "value": cfg.CDNDomain},
						{"name": "PORT", "value": "8080"},
						{"name": "ENV", "value": "production"},
						{"name": "MONGO_URI", "value": mongoURI},
						{"name": "JWT_SECRET", "value": jwtSecret},
					},
					"logConfiguration": map[string]interface{}{
						"logDriver": "awslogs",
						"options": map[string]string{
							"awslogs-group":         "/image-pipeline/api",
							"awslogs-region":        cfg.AWSRegion,
							"awslogs-stream-prefix": "api",
						},
					},
					"essential": true,
					"healthCheck": map[string]interface{}{
						"command":     []string{"CMD-SHELL", "wget -qO- http://localhost:8080/health || exit 1"},
						"interval":    30,
						"timeout":     5,
						"retries":     3,
						"startPeriod": 60,
					},
				},
				// Alloy sidecar
				{
					"name":  "alloy",
					"image": cfg.AlloyImageURI,
					"environment": []map[string]string{
						{"name": "GRAFANA_API_KEY", "value": grafanaAPIKey},
					},
					"logConfiguration": map[string]interface{}{
						"logDriver": "awslogs",
						"options": map[string]string{
							"awslogs-group":         "/image-pipeline/api",
							"awslogs-region":        cfg.AWSRegion,
							"awslogs-stream-prefix": "alloy",
						},
					},
					"essential": false,
					"dependsOn": []map[string]string{
						{"containerName": "api", "condition": "HEALTHY"},
					},
				},
			}

			b, err := json.Marshal(def)
			return string(b), err
		},
	).(pulumi.StringOutput)

	return ecs.NewTaskDefinition(ctx, "api-task", &ecs.TaskDefinitionArgs{
		Family:                  pulumi.String("image-pipeline-api"),
		NetworkMode:             pulumi.String("awsvpc"),
		RequiresCompatibilities: pulumi.StringArray{pulumi.String("FARGATE")},
		Cpu:                     pulumi.String("256"), // 0.25 vCPU
		Memory:                  pulumi.String("512"), // 512MB
		ExecutionRoleArn:        roles.TaskExecutionRole.Arn,
		TaskRoleArn:             roles.TaskRole.Arn,
		ContainerDefinitions:    containerDef,
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-api"),
		},
	})
}

func createWorkerTaskDef(
	ctx *pulumi.Context,
	roles *IAMRoles,
	logs *LogGroups,
	cfg *AppConfig,
) (*ecs.TaskDefinition, error) {

	containerDef := pulumi.All(cfg.MongoURI, cfg.JWTSecret).ApplyT(
		func(args []interface{}) (string, error) {
			mongoURI := args[0].(string)
			jwtSecret := args[1].(string)

			def := []map[string]interface{}{
				{
					"name":  "worker",
					"image": cfg.WorkerImageURI,
					"environment": []map[string]string{
						{"name": "AWS_REGION", "value": cfg.AWSRegion},
						{"name": "S3_BUCKET", "value": cfg.S3Bucket},
						{"name": "SQS_QUEUE_URL", "value": cfg.SQSQueueURL},
						{"name": "CDN_DOMAIN", "value": cfg.CDNDomain},
						{"name": "WORKER_COUNT", "value": cfg.WorkerCount},
						{"name": "ENV", "value": "production"},
						{"name": "MONGO_URI", "value": mongoURI},
						{"name": "JWT_SECRET", "value": jwtSecret},
					},
					"logConfiguration": map[string]interface{}{
						"logDriver": "awslogs",
						"options": map[string]string{
							"awslogs-group":         "/image-pipeline/worker",
							"awslogs-region":        cfg.AWSRegion,
							"awslogs-stream-prefix": "worker",
						},
					},
					"essential": true,
				},
			}

			b, err := json.Marshal(def)
			return string(b), err
		},
	).(pulumi.StringOutput)

	return ecs.NewTaskDefinition(ctx, "worker-task", &ecs.TaskDefinitionArgs{
		Family:                  pulumi.String("image-pipeline-worker"),
		NetworkMode:             pulumi.String("awsvpc"),
		RequiresCompatibilities: pulumi.StringArray{pulumi.String("FARGATE")},
		Cpu:                     pulumi.String("256"),
		Memory:                  pulumi.String("512"),
		ExecutionRoleArn:        roles.TaskExecutionRole.Arn,
		TaskRoleArn:             roles.TaskRole.Arn,
		ContainerDefinitions:    containerDef,
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-worker"),
		},
	})
}
