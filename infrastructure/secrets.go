package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/secretsmanager"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type SecretsResources struct {
	Secret *secretsmanager.Secret
}

func createSecrets(
	ctx *pulumi.Context,
	roles *IAMRoles,
	mongoURI pulumi.StringOutput,
	jwtSecret pulumi.StringOutput,
	grafanaAPIKey pulumi.StringOutput,
) (*SecretsResources, error) {
	secret, err := secretsmanager.NewSecret(ctx, "app-secrets", &secretsmanager.SecretArgs{
		Name: pulumi.String("image-pipeline/prod/secrets"),
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-secrets"),
		},
	})
	if err != nil {
		return nil, err
	}

	// Build JSON secret value from Pulumi config secrets
	secretValue := pulumi.All(mongoURI, jwtSecret, grafanaAPIKey).ApplyT(
		func(args []any) (string, error) {
			mongo := args[0].(string)
			jwt := args[1].(string)
			grafana := args[2].(string)
			return fmt.Sprintf(`{"MONGO_URI":"%s","JWT_SECRET":"%s","GRAFANA_API_KEY":"%s"}`,
				mongo, jwt, grafana), nil
		},
	).(pulumi.StringOutput)

	_, err = secretsmanager.NewSecretVersion(ctx, "app-secrets-version", &secretsmanager.SecretVersionArgs{
		SecretId:     secret.ID(),
		SecretString: secretValue,
	})
	if err != nil {
		return nil, err
	}

	// Grant execution role permission to read the secret
	policy := secret.Arn.ApplyT(func(arn string) string {
		return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "secretsmanager:GetSecretValue",
      "Resource": "%s"
    }
  ]
}`, arn)
	}).(pulumi.StringOutput)

	_, err = iam.NewRolePolicy(ctx, "secrets-access-policy", &iam.RolePolicyArgs{
		Name:   pulumi.String("image-pipeline-secrets-access"),
		Role:   roles.TaskExecutionRole.Name,
		Policy: policy,
	})
	if err != nil {
		return nil, err
	}

	ctx.Export("secretArn", secret.Arn)

	return &SecretsResources{
		Secret: secret,
	}, nil
}
