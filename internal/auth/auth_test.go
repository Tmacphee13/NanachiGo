package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	// "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/joho/godotenv"
)

// TestAWSAuthentication tests if AWS authentication works using the .env file
func TestAWSAuthentication(t *testing.T) {

	//
	envPath := filepath.Join("..", "..", ".env")

	// Load environment variables from the .env file in project root
	err := godotenv.Load(envPath)
	if err != nil {
		t.Fatalf("Failed to load .env file: %v", err)
	}

	// Ensure critical environment variables are set
	if os.Getenv("AWS_REGION") == "" {
		t.Fatal("AWS_REGION is not set in the .env file")
	}
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" {
		t.Fatal("AWS_ACCESS_KEY_ID is not set in the .env file")
	}
	if os.Getenv("AWS_SECRET_ACCESS_KEY") == "" {
		t.Fatal("AWS_SECRET_ACCESS_KEY is not set in the .env file")
	}
	if os.Getenv("AWS_SESSION_TOKEN") == "" {
		t.Fatal("AWS_SESSION_TOKEN is not set in the .env file")
	}

	cfg, err := config.LoadDefaultConfig(context.TODO())

	// Use STS to test authentication
	client := sts.NewFromConfig(cfg)
	out, err := client.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
	if err != nil {
		t.Fatalf("Failed to authenticate with AWS: %v", err)
	}

	t.Log("Successfully authenticated with AWS")
	t.Logf("Account ID: %s\n", *out.Account)
	t.Logf("User/Role: %s\n", *out.Arn)
	t.Logf("User ID: %s\n", *out.UserId)
}
