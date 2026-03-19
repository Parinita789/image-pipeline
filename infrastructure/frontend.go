package main

import (
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/apigatewayv2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/cloudfront"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type FrontendResources struct {
	Bucket       *s3.BucketV2
	Distribution *cloudfront.Distribution
}

func createFrontend(ctx *pulumi.Context, apiGateway *apigatewayv2.Api) (*FrontendResources, error) {
	callerIdentity, err := aws.GetCallerIdentity(ctx, &aws.GetCallerIdentityArgs{})
	if err != nil {
		return nil, err
	}

	bucket, err := s3.NewBucketV2(ctx, "frontend-bucket", &s3.BucketV2Args{
		Bucket: pulumi.Sprintf("image-pipeline-frontend-%s", callerIdentity.AccountId),
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-frontend"),
		},
	})
	if err != nil {
		return nil, err
	}

	// Block all public access — CloudFront OAC handles it
	_, err = s3.NewBucketPublicAccessBlock(ctx, "frontend-public-access-block", &s3.BucketPublicAccessBlockArgs{
		Bucket:                bucket.ID(),
		BlockPublicAcls:       pulumi.Bool(true),
		BlockPublicPolicy:     pulumi.Bool(true),
		IgnorePublicAcls:      pulumi.Bool(true),
		RestrictPublicBuckets: pulumi.Bool(true),
	})
	if err != nil {
		return nil, err
	}

	// Origin Access Control for CloudFront → S3
	oac, err := cloudfront.NewOriginAccessControl(ctx, "frontend-oac", &cloudfront.OriginAccessControlArgs{
		Name:                          pulumi.String("image-pipeline-frontend-oac"),
		OriginAccessControlOriginType: pulumi.String("s3"),
		SigningBehavior:               pulumi.String("always"),
		SigningProtocol:               pulumi.String("sigv4"),
	})
	if err != nil {
		return nil, err
	}

	// CloudFront Function to strip /api prefix before forwarding to API Gateway
	cfFunctionStripApi, err := cloudfront.NewFunction(ctx, "strip-api-prefix", &cloudfront.FunctionArgs{
		Name:    pulumi.String("image-pipeline-strip-api-prefix"),
		Runtime: pulumi.String("cloudfront-js-2.0"),
		Code: pulumi.String(`function handler(event) {
  var request = event.request;
  request.uri = request.uri.replace(/^\/api/, '');
  if (request.uri === '') {
    request.uri = '/';
  }
  return request;
}`),
	})
	if err != nil {
		return nil, err
	}

	// CloudFront Function for SPA routing — rewrites non-file paths to /index.html
	cfFunctionSPA, err := cloudfront.NewFunction(ctx, "spa-rewrite", &cloudfront.FunctionArgs{
		Name:    pulumi.String("image-pipeline-spa-rewrite"),
		Runtime: pulumi.String("cloudfront-js-2.0"),
		Code: pulumi.String(`function handler(event) {
  var request = event.request;
  if (!request.uri.includes('.')) {
    request.uri = '/index.html';
  }
  return request;
}`),
	})
	if err != nil {
		return nil, err
	}

	// Extract API Gateway domain from endpoint URL
	apiDomain := apiGateway.ApiEndpoint.ApplyT(func(endpoint string) string {
		return strings.TrimPrefix(endpoint, "https://")
	}).(pulumi.StringOutput)

	s3OriginID := "s3-frontend"
	apiOriginID := "api-gateway"

	distribution, err := cloudfront.NewDistribution(ctx, "frontend-distribution", &cloudfront.DistributionArgs{
		Enabled:           pulumi.Bool(true),
		DefaultRootObject: pulumi.String("index.html"),
		PriceClass:        pulumi.String("PriceClass_100"),

		Origins: cloudfront.DistributionOriginArray{
			&cloudfront.DistributionOriginArgs{
				OriginId:              pulumi.String(s3OriginID),
				DomainName:            bucket.BucketRegionalDomainName,
				OriginAccessControlId: oac.ID(),
			},
			&cloudfront.DistributionOriginArgs{
				OriginId:   pulumi.String(apiOriginID),
				DomainName: apiDomain,
				CustomOriginConfig: &cloudfront.DistributionOriginCustomOriginConfigArgs{
					HttpPort:             pulumi.Int(80),
					HttpsPort:            pulumi.Int(443),
					OriginProtocolPolicy: pulumi.String("https-only"),
					OriginSslProtocols:   pulumi.StringArray{pulumi.String("TLSv1.2")},
				},
			},
		},

		DefaultCacheBehavior: &cloudfront.DistributionDefaultCacheBehaviorArgs{
			TargetOriginId:       pulumi.String(s3OriginID),
			ViewerProtocolPolicy: pulumi.String("redirect-to-https"),
			AllowedMethods:       pulumi.StringArray{pulumi.String("GET"), pulumi.String("HEAD")},
			CachedMethods:        pulumi.StringArray{pulumi.String("GET"), pulumi.String("HEAD")},
			Compress:             pulumi.Bool(true),
			CachePolicyId:        pulumi.String("658327ea-f89d-4fab-a63d-7e88639e58f6"),
			FunctionAssociations: cloudfront.DistributionDefaultCacheBehaviorFunctionAssociationArray{
				&cloudfront.DistributionDefaultCacheBehaviorFunctionAssociationArgs{
					EventType:   pulumi.String("viewer-request"),
					FunctionArn: cfFunctionSPA.Arn,
				},
			},
		},

		OrderedCacheBehaviors: cloudfront.DistributionOrderedCacheBehaviorArray{
			&cloudfront.DistributionOrderedCacheBehaviorArgs{
				PathPattern:          pulumi.String("/api/*"),
				TargetOriginId:       pulumi.String(apiOriginID),
				ViewerProtocolPolicy: pulumi.String("redirect-to-https"),
				AllowedMethods: pulumi.StringArray{
					pulumi.String("GET"), pulumi.String("HEAD"), pulumi.String("OPTIONS"),
					pulumi.String("PUT"), pulumi.String("POST"), pulumi.String("PATCH"), pulumi.String("DELETE"),
				},
				CachedMethods:         pulumi.StringArray{pulumi.String("GET"), pulumi.String("HEAD")},
				CachePolicyId:         pulumi.String("4135ea2d-6df8-44a3-9df3-4b5a84be39ad"),
				OriginRequestPolicyId: pulumi.String("b689b0a8-53d0-40ab-baf2-68738e2966ac"),
				FunctionAssociations: cloudfront.DistributionOrderedCacheBehaviorFunctionAssociationArray{
					&cloudfront.DistributionOrderedCacheBehaviorFunctionAssociationArgs{
						EventType:   pulumi.String("viewer-request"),
						FunctionArn: cfFunctionStripApi.Arn,
					},
				},
			},
		},

		Restrictions: &cloudfront.DistributionRestrictionsArgs{
			GeoRestriction: &cloudfront.DistributionRestrictionsGeoRestrictionArgs{
				RestrictionType: pulumi.String("none"),
			},
		},

		ViewerCertificate: &cloudfront.DistributionViewerCertificateArgs{
			CloudfrontDefaultCertificate: pulumi.Bool(true),
		},

		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-frontend"),
		},
	})
	if err != nil {
		return nil, err
	}

	// Bucket policy granting CloudFront OAC access
	bucketPolicy := pulumi.All(bucket.Arn, distribution.Arn).ApplyT(
		func(args []interface{}) (string, error) {
			bucketArn := args[0].(string)
			distArn := args[1].(string)

			policy := fmt.Sprintf(`{
  				"Version": "2012-10-17",
 				"Statement": [
    				{
      					"Sid": "AllowCloudFrontServicePrincipalReadOnly",
      					"Effect": "Allow",
      					"Principal": {
       						"Service": "cloudfront.amazonaws.com"
      					},
      			"Action": "s3:GetObject",
      			"Resource": "%s/*",
      			"Condition": 
					{
        				"StringEquals": {
          					"AWS:SourceArn": "%s"
        				}
      				}
   				}
			]}`,
				bucketArn, distArn)
			return policy, nil
		},
	).(pulumi.StringOutput)

	_, err = s3.NewBucketPolicy(ctx, "frontend-bucket-policy", &s3.BucketPolicyArgs{
		Bucket: bucket.ID(),
		Policy: bucketPolicy,
	})
	if err != nil {
		return nil, err
	}

	// Exports
	ctx.Export("frontendBucketName", bucket.Bucket)
	ctx.Export("frontendUrl", pulumi.Sprintf("https://%s", distribution.DomainName))
	ctx.Export("distributionId", distribution.ID())

	return &FrontendResources{
		Bucket:       bucket,
		Distribution: distribution,
	}, nil
}
