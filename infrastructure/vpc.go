package main

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type VPCResources struct {
	VPC           *ec2.Vpc
	PublicSubnet  *ec2.Subnet
	SecurityGroup *ec2.SecurityGroup
}

func createVPC(ctx *pulumi.Context) (*VPCResources, error) {
	vpc, err := ec2.NewVpc(ctx, "image-pipeline-vpc", &ec2.VpcArgs{
		CidrBlock:          pulumi.String("10.0.0.0/16"),
		EnableDnsHostnames: pulumi.Bool(true),
		EnableDnsSupport:   pulumi.Bool(true),
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-vpc"),
		},
	})
	if err != nil {
		return nil, err
	}

	// internet gateway
	igw, err := ec2.NewInternetGateway(ctx, "image-pipeline-igw", &ec2.InternetGatewayArgs{
		VpcId: vpc.ID(),
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-igw"),
		},
	})
	if err != nil {
		return nil, err
	}

	// Public Subnet
	subnet, err := ec2.NewSubnet(ctx, "image-pipeline-subnet", &ec2.SubnetArgs{
		VpcId:               vpc.ID(),
		CidrBlock:           pulumi.String("10.0.1.0/24"),
		AvailabilityZone:    pulumi.String("us-west-1a"),
		MapPublicIpOnLaunch: pulumi.Bool(true),
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-public-subnet"),
		},
	})
	if err != nil {
		return nil, err
	}

	// Route Table
	rt, err := ec2.NewRouteTable(ctx, "image-pipeline-rt", &ec2.RouteTableArgs{
		VpcId: vpc.ID(),
		Routes: ec2.RouteTableRouteArray{
			&ec2.RouteTableRouteArgs{
				CidrBlock: pulumi.String("0.0.0.0/0"),
				GatewayId: igw.ID(),
			},
		},
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-rt"),
		},
	})
	if err != nil {
		return nil, err
	}

	// associate route table with subnet
	_, err = ec2.NewRouteTableAssociation(ctx, "image-pipeline-rta", &ec2.RouteTableAssociationArgs{
		SubnetId:     subnet.ID(),
		RouteTableId: rt.ID(),
	})
	if err != nil {
		return nil, err
	}

	// Security Group
	sg, err := ec2.NewSecurityGroup(ctx, "image-pipeline-sg", &ec2.SecurityGroupArgs{
		VpcId:       vpc.ID(),
		Description: pulumi.String("image pipeline ECS security group"),
		Ingress: ec2.SecurityGroupIngressArray{
			// API
			&ec2.SecurityGroupIngressArgs{
				Protocol:   pulumi.String("tcp"),
				FromPort:   pulumi.Int(8080),
				ToPort:     pulumi.Int(8080),
				CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				// PrefixListIds: pulumi.StringArray{
				// 	pulumi.String("pl-00a54069"),
				// },
				Description: pulumi.String("API Gateway only"),
			},
			// Prometheus metrics port
			// &ec2.SecurityGroupIngressArgs{
			// 	Protocol:   pulumi.String("tcp"),
			// 	FromPort:   pulumi.Int(8080),
			// 	ToPort:     pulumi.Int(8080),
			// 	CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			// 	// Ipv6CidrBlocks: pulumi.StringArray{pulumi.String("::/0")},
			// 	Description: pulumi.String("API port IPv6"),
			// },
		},
		Egress: ec2.SecurityGroupEgressArray{
			// allow all outbound — needed for S3, SQS, MongoDB Atlas, ECR
			&ec2.SecurityGroupEgressArgs{
				Protocol:    pulumi.String("-1"),
				FromPort:    pulumi.Int(0),
				ToPort:      pulumi.Int(0),
				CidrBlocks:  pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				Description: pulumi.String("all outbound"),
			},
		},
		Tags: pulumi.StringMap{
			"Name": pulumi.String("image-pipeline-sg"),
		},
	})
	if err != nil {
		return nil, err
	}

	ctx.Export("vpcId", vpc.ID())
	ctx.Export("subnetId", subnet.ID())
	ctx.Export("securityGroupId", sg.ID())

	return &VPCResources{
		VPC:           vpc,
		PublicSubnet:  subnet,
		SecurityGroup: sg,
	}, nil

}
