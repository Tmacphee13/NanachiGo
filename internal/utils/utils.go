package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
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

func UploadPaper(w http.ResponseWriter, r *http.Request) {
	// Placeholder implementation
}

func ExtractMetadata(ctx context.Context, client *bedrockruntime.Client, pdfText string) (map[string]interface{}, error) {
	// Define the system-level prompt for Claude
	systemPrompt := `You are a research paper analyzer. Extract the title, all authors, and publication date from research papers. Return only valid JSON with no additional text.`

	// Define the user-level prompt to extract metadata
	// Limit text to the first 4000 characters to fit the model's token limit
	prompt := fmt.Sprintf(`Extract the title, all authors, and the publication date from the following research paper text. The date might be just a month and year, or more specific. Return only a JSON object with the following structure:
{
  "title": "paper title",
  "authors": ["author1", "author2"],
  "date": "publication date"
}

Text:

%s`, pdfText[:int(math.Min(float64(len(pdfText)), 4000))])

	// Call Claude with the provided prompts
	response, err := CallClaude(ctx, client, prompt, systemPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to call Claude: %w", err)
	}

	return response, nil
}

func GenerateMindmap(ctx context.Context, client *bedrockruntime.Client, pdfText string) (map[string]interface{}, error) {
	// Define the system-level prompt for Claude
	systemPrompt := `You are an expert at creating hierarchical mind maps from academic papers. Create structured JSON mind maps with up to 8 levels of depth. Each node must have: name, tooltip, section, pages, and optionally children. Return only valid JSON with no additional text.`

	// Define the user-level prompt to generate a mind map
	// No need to truncate text here, as Claude’s token limit will likely be handled at the API level or by CallClaude
	prompt := fmt.Sprintf(`Analyze the following research paper text and create a hierarchical mind map summarizing its key concepts. The structure should be a nested JSON object with up to 8 levels but start with no more than 5.

For each node, provide:
- 'name': concise topic name
- 'tooltip': three to five sentences, plain-english explanation, summarization of content
- 'section': the document section it belongs to (e.g., "Introduction", "2.1 Related Work")
- 'pages': a string with the source page number(s) (e.g., "3" or "5-7" - these must be factually accurate)
- 'children': array of child nodes (if applicable)

The root object should represent the paper's main theme and must have a 'children' array.

Return the response as a JSON object in this exact format:
{
  "name": "main topic",
  "tooltip": "explanation",
  "section": "section name",
  "pages": "page numbers",
  "children": [
    {
      "name": "subtopic",
      "tooltip": "explanation",
      "section": "section name",
      "pages": "page numbers",
      "children": [...]
    }
  ]
}

Here is the text:

%s`, pdfText)

	// Call Claude with the provided prompts
	response, err := CallClaude(ctx, client, prompt, systemPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to call Claude: %w", err)
	}

	return response, nil
}

func CallClaude(ctx context.Context, client *bedrockruntime.Client, prompt, systemPrompt string) (map[string]interface{}, error) {
	modelID := "anthropic.claude-3-5-haiku-20241022-v1:0" // Claude 3.5 Haiku

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
