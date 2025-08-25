package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// Thought we were going to need to use? But I guess not
// https://docs.aws.amazon.com/code-library/latest/ug/go_2_bedrock-runtime_code_examples.html
type InvokeModelWrapper struct {
	BedrockClient *bedrockruntime.Client
}

// ClaudeRequest represents the request payload for Claude
type ClaudeRequest struct {
	AnthropicVersion string    `json:"anthropic_version"`
	MaxTokens        int       `json:"max_tokens"`
	Temperature      float64   `json:"temperature"`
	System           string    `json:"system,omitempty"`
	Messages         []Message `json:"messages"`
}

// Message represents a message in the conversation
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ClaudeResponse represents the response from Claude
type ClaudeResponse struct {
	Content []Content `json:"content"`
}

// Content represents the content in Claude's response
type Content struct {
	Text string `json:"text"`
}

// CallClaude calls Claude on AWS Bedrock with retry logic and exponential backoff
func CallClaude(ctx context.Context, client *bedrockruntime.Client, prompt, systemPrompt string) (map[string]interface{}, error) {
	modelID := "anthropic.claude-3-5-haiku-20241022-v1:0" // Claude 3 Haiku

	payload := ClaudeRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        4000,
		Temperature:      0.0,
		System:           systemPrompt,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	const maxRetries = 3
	delay := time.Second // Start with a 1-second delay

	for i := range maxRetries {
		input := &bedrockruntime.InvokeModelInput{
			ModelId:     aws.String(modelID),
			ContentType: aws.String("application/json"),
			Accept:      aws.String("application/json"),
			Body:        payloadBytes,
		}

		response, err := client.InvokeModel(ctx, input)
		if err != nil {
			log.Printf("Bedrock API error (attempt %d): %v", i+1, err)

			// Check for throttling or service errors
			errStr := err.Error()
			if strings.Contains(errStr, "ThrottlingException") || strings.Contains(errStr, "ServiceException") {
				if i < maxRetries-1 {
					log.Printf("Retrying in %v...", delay)
					time.Sleep(delay)
					delay *= 2 // Exponential backoff
					continue
				}
			}

			// For the last retry or non-retryable errors, return error
			if i == maxRetries-1 {
				log.Printf("Error calling Bedrock Claude API after all retries: %v", err)
				return nil, fmt.Errorf("bedrock API call failed after %d retries: %w", maxRetries, err)
			}
			continue
		}

		// Parse the response
		var responseBody ClaudeResponse
		if err := json.Unmarshal(response.Body, &responseBody); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		if len(responseBody.Content) == 0 {
			return nil, fmt.Errorf("empty response content")
		}

		responseText := responseBody.Content[0].Text

		// Try to parse as JSON
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(responseText), &result); err == nil {
			return result, nil
		}

		// If JSON parsing fails, try to extract JSON from the text
		re := regexp.MustCompile(`\{[\s\S]*\}`)
		jsonMatch := re.FindString(responseText)
		if jsonMatch != "" {
			if err := json.Unmarshal([]byte(jsonMatch), &result); err == nil {
				return result, nil
			}
		}

		return nil, fmt.Errorf("could not parse JSON from Claude response: %s", responseText)
	}

	return nil, fmt.Errorf("bedrock Claude API call failed after multiple retries")
}
