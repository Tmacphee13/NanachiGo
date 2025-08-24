package auth

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// GetAWSConfig creates an AWS configuration using environment variables
func GetAWSConfig() (aws.Config, error) {

	// Get the AWS region from environment variables
	region := os.Getenv("AWS_REGION")
	if region == "" {
		log.Fatalf("AWS_REGION is not set in the environment")
	}

	// Configure AWS credentials from environment variables
	cfg, err := config.LoadDefaultConfig(context.TODO())

	if err != nil {
		log.Fatalf("Unable to load AWS configuration: %v", err)
	}

	return cfg, err
}

// TestAuthentication tests AWS authentication using the STS GetCallerIdentity API
func TestAuthentication(cfg aws.Config) {
	client := sts.NewFromConfig(cfg)

	// Call the GetCallerIdentity API
	output, err := client.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
	if err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}

	// Print authentication success details
	fmt.Println("Authentication successful!")
	fmt.Printf("Account ID: %s\n", *output.Account)
	fmt.Printf("User/Role ARN: %s\n", *output.Arn)
	fmt.Printf("User ID: %s\n", *output.UserId)
}
