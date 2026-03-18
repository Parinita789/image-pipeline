package main

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/apigatewayv2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func createAPIGateway(ctx *pulumi.Context, services *ECSServices) (*apigatewayv2.Api, error) {
	cfg := config.New(ctx, "")
	taskIp := cfg.Require("apiTaskIp")

	api, err := apigatewayv2.NewApi(ctx, "image-pipeline-api-gw", &apigatewayv2.ApiArgs{
		Name:         pulumi.String("image-pipeline"),
		ProtocolType: pulumi.String("HTTP"),
		CorsConfiguration: &apigatewayv2.ApiCorsConfigurationArgs{
			AllowOrigins: pulumi.StringArray{pulumi.String("*")},
			AllowMethods: pulumi.StringArray{
				pulumi.String("GET"),
				pulumi.String("POST"),
				pulumi.String("DELETE"),
				pulumi.String("OPTIONS"),
			},
			AllowHeaders: pulumi.StringArray{
				pulumi.String("Content-Type"),
				pulumi.String("Authorization"),
				pulumi.String("X-Idempotency-Key"),
				pulumi.String("X-Request-ID"),
			},
		},
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline"),
		},
	})
	if err != nil {
		return nil, err
	}

	// forward all requests to ECS API task
	integration, err := apigatewayv2.NewIntegration(ctx, "ecs-integration", &apigatewayv2.IntegrationArgs{
		ApiId:             api.ID(),
		IntegrationType:   pulumi.String("HTTP_PROXY"),
		IntegrationMethod: pulumi.String("ANY"),
		IntegrationUri:    pulumi.Sprintf("http://%s:8080", taskIp), PayloadFormatVersion: pulumi.String("1.0"),
	})
	if err != nil {
		return nil, err
	}

	_, err = apigatewayv2.NewRoute(ctx, "catch-all-route", &apigatewayv2.RouteArgs{
		ApiId:    api.ID(),
		RouteKey: pulumi.String("ANY /{proxy+}"),
		Target:   pulumi.Sprintf("integrations/%s", integration.ID()),
	})
	if err != nil {
		return nil, err
	}

	// auto deploy stage
	_, err = apigatewayv2.NewStage(ctx, "prod-stage", &apigatewayv2.StageArgs{
		ApiId:      api.ID(),
		Name:       pulumi.String("$default"),
		AutoDeploy: pulumi.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	// temporarily add this to apigateway.go
	ctx.Export("integrationUri", pulumi.Sprintf("http://%s:8080", taskIp))
	ctx.Export("apiGatewayUrl", api.ApiEndpoint)
	ctx.Export("apiGatewayId", api.ID())

	return api, nil
}
