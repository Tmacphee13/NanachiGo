package auth

import (
    "context"
    "errors"
    "fmt"
    "log"
    "os"
    "strings"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/sts"
)

// GetAWSConfig creates an AWS configuration using environment variables
func GetAWSConfig() (aws.Config, error) {
    // Read/validate region first
    region := strings.TrimSpace(os.Getenv("AWS_REGION"))
    if region == "" {
        log.Printf("aws: missing AWS_REGION; set it to your target region (e.g., us-east-1)")
        return aws.Config{}, errors.New("AWS_REGION not set")
    }

    // Log presence of common credential envs (without secrets)
    hasKey := os.Getenv("AWS_ACCESS_KEY_ID") != ""
    hasSecret := os.Getenv("AWS_SECRET_ACCESS_KEY") != ""
    hasSession := os.Getenv("AWS_SESSION_TOKEN") != ""
    profile := strings.TrimSpace(os.Getenv("AWS_PROFILE"))
    log.Printf("aws: loading config (region=%s, key=%t, secret=%t, session_token=%t, profile=%s)", region, hasKey, hasSecret, hasSession, profile)

    // Load configuration, preferring provided region
    cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
    if err != nil {
        return aws.Config{}, fmt.Errorf("aws: failed loading default config: %w", err)
    }
    return cfg, nil
}

// TestAuthentication tests AWS authentication using the STS GetCallerIdentity API
func TestAuthentication(cfg aws.Config) {
    client := sts.NewFromConfig(cfg)
    output, err := client.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
    if err != nil {
        log.Printf("aws: STS GetCallerIdentity failed: %v", err)
        return
    }
    log.Printf("aws: authentication OK (account=%s, arn=%s)", aws.ToString(output.Account), aws.ToString(output.Arn))
}
