package main

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ECSCluster struct {
	Cluster *ecs.Cluster
}

func createECSCluster(ctx *pulumi.Context) (*ECSCluster, error) {
	cluster, err := ecs.NewCluster(ctx, "image-pipeline-cluster", &ecs.ClusterArgs{
		Name: pulumi.String("image-pipeline"),
		Settings: ecs.ClusterSettingArray{
			&ecs.ClusterSettingArgs{
				Name:  pulumi.String("containerInsights"),
				Value: pulumi.String("enabled"),
			},
		},
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline"),
		},
	})
	if err != nil {
		return nil, err
	}

	ctx.Export("clusterName", cluster.Name)
	ctx.Export("clusterArn", cluster.Arn)

	return &ECSCluster{Cluster: cluster}, nil
}
