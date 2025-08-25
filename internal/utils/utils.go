package utils

import (
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

const MODEL_ID string = "anthropic.claude-3-5-haiku-20241022-v1:0"

type InvokeModelWrapper struct {
	BedrockClient *bedrockruntime.Client
}

type ClaudeRequest struct {
	Prompt            string   `json:"prompt"`
	MaxTokensToSample int      `json:"max_tokens_to_sample"`
	Temperature       float64  `json:"temperature,omitempty"`
	StopSequences     []string `json:"stop_sequences,omitempty"`
}

type ClaudeResponse struct {
	Completion string `json:"completion"`
}

type Metadata struct {
	Title string
}

func UploadPaper(w http.ResponseWriter, r *http.Request) {

}

func CallClaude(prompt string, system_prompt string) (string, error) {
	// Placeholder implementation
	return "This is a response from Claude.", nil

}
