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
    "os"
    "io"
    "path/filepath"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
    pdfread "github.com/ledongthuc/pdf"
    "github.com/google/uuid"
    "github.com/Tmacphee13/NanachiGo/internal/db"
    "github.com/Tmacphee13/NanachiGo/internal/auth"
    genai "github.com/google/generative-ai-go/genai"
    "google.golang.org/api/option"
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
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    platform := r.URL.Query().Get("platform")
    if platform == "" { platform = defaultPlatform() }
    log.Printf("upload: starting PDF upload (platform=%s)", platform)

    // Parse multipart form (allow up to ~25MB)
    if err := r.ParseMultipartForm(25 << 20); err != nil {
        http.Error(w, "failed to parse form", http.StatusBadRequest)
        return
    }
    file, header, err := r.FormFile("pdf")
    if err != nil {
        log.Printf("upload: no file uploaded: %v", err)
        http.Error(w, "no file uploaded", http.StatusBadRequest)
        return
    }
    defer file.Close()
    log.Printf("upload: received file %q", header.Filename)

    // Read file bytes to temp path for pdf parser
    tmpDir := os.TempDir()
    tmpPath := filepath.Join(tmpDir, fmt.Sprintf("upload-%s.pdf", uuid.New().String()))
    out, err := os.Create(tmpPath)
    if err != nil {
        log.Printf("upload: failed to create temp file: %v", err)
        http.Error(w, "failed to create temp file", http.StatusInternalServerError)
        return
    }
    if _, err := io.Copy(out, file); err != nil {
        out.Close()
        os.Remove(tmpPath)
        log.Printf("upload: failed to write temp file: %v", err)
        http.Error(w, "failed to write temp file", http.StatusInternalServerError)
        return
    }
    out.Close()
    defer os.Remove(tmpPath)

    // Extract PDF text
    pdfFile, rdr, err := pdfread.Open(tmpPath)
    if err != nil {
        log.Printf("upload: failed to read pdf: %v", err)
        http.Error(w, "failed to read pdf", http.StatusInternalServerError)
        return
    }
    defer pdfFile.Close()
    var buf strings.Builder
    totalPage := rdr.NumPage()
    for pageIndex := 1; pageIndex <= totalPage; pageIndex++ {
        p := rdr.Page(pageIndex)
        if p.V.IsNull() { continue }
        content, _ := p.GetPlainText(nil)
        buf.WriteString(content)
        buf.WriteString("\n")
    }
    pdfText := buf.String()

    ctx := r.Context()
    var metadata map[string]interface{}
    var mindmapData map[string]interface{}
    switch platform {
    case "aws":
        brClient, err := NewBedrockClient()
        if err != nil {
            log.Printf("aws: bedrock init failed: %v", err)
            http.Error(w, "failed to init bedrock", http.StatusInternalServerError)
            return
        }
        metadata, err = ExtractMetadata(ctx, brClient, pdfText)
        if err != nil { log.Printf("metadata error: %v", err); http.Error(w, "failed to extract metadata", http.StatusInternalServerError); return }
        mindmapData, err = GenerateMindmap(ctx, brClient, pdfText)
        if err != nil { log.Printf("mindmap error: %v", err); http.Error(w, "failed to generate mindmap", http.StatusInternalServerError); return }
    case "gcp":
        gmClient, err := NewGeminiClient(ctx)
        if err != nil {
            log.Printf("gcp: gemini init failed: %v", err)
            http.Error(w, "failed to init gemini", http.StatusInternalServerError)
            return
        }
        // Close Gemini client after we're done with both calls
        defer gmClient.Close()
        metadata, err = ExtractMetadataGemini(ctx, gmClient, pdfText)
        if err != nil { log.Printf("metadata error: %v", err); http.Error(w, "failed to extract metadata", http.StatusInternalServerError); return }
        mindmapData, err = GenerateMindmapGemini(ctx, gmClient, pdfText)
        if err != nil { log.Printf("mindmap error: %v", err); http.Error(w, "failed to generate mindmap", http.StatusInternalServerError); return }
    default:
        http.Error(w, "unknown platform", http.StatusBadRequest)
        return
    }

    // Normalize fields from metadata
    title, _ := metadata["title"].(string)
    title = strings.TrimSpace(title)
    if title == "" {
        // Fallback to filename without extension
        base := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
        title = base
    }
    date, _ := metadata["date"].(string)
    var authors []string
    if arr, ok := metadata["authors"].([]interface{}); ok {
        for _, a := range arr {
            if s, ok := a.(string); ok {
                authors = append(authors, s)
            }
        }
    } else if arrs, ok := metadata["authors"].([]string); ok {
        authors = arrs
    }

    now := time.Now().UTC().Format(time.RFC3339)
    item := db.MindmapItem{
        ID:          uuid.New().String(),
        Filename:    header.Filename,
        Title:       title,
        Authors:     authors,
        Date:        date,
        MindmapData: mindmapData,
        PDFText:     pdfText,
        CreatedAt:   now,
        UpdatedAt:   now,
    }

    id, err := db.CreateMindmapPlatform(ctx, platform, item)
    if err != nil {
        log.Printf("db: create mindmap failed: %v", err)
        http.Error(w, "failed to store mindmap", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    fmt.Fprintf(w, `{"success":true,"message":"PDF processed and mind map created!","mindmapId":"%s"}` , id)
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

// NewBedrockClient creates a Bedrock runtime client using shared AWS config
func NewBedrockClient() (*bedrockruntime.Client, error) {
    awsCfg, err := auth.GetAWSConfig()
    if err != nil {
        log.Printf("aws: config error: %v", err)
        return nil, err
    }
    return bedrockruntime.NewFromConfig(awsCfg), nil
}

// --------------- Gemini Support (GCP) --------------- //

func NewGeminiClient(ctx context.Context) (*genai.Client, error) {
    apiKey := os.Getenv("GEMINI_API_KEY")
    if apiKey == "" {
        log.Printf("gcp: GEMINI_API_KEY not set")
        return nil, fmt.Errorf("GEMINI_API_KEY not set")
    }
    return genai.NewClient(ctx, option.WithAPIKey(apiKey))
}

func CallGemini(ctx context.Context, client *genai.Client, prompt, systemPrompt string) (map[string]interface{}, error) {
    model := client.GenerativeModel("gemini-1.5-flash")
    // Combine system + user prompts to keep logic simple
    fullPrompt := systemPrompt + "\n\n" + prompt
    resp, err := model.GenerateContent(ctx, genai.Text(fullPrompt))
    if err != nil {
        return nil, err
    }
    if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
        return nil, fmt.Errorf("empty response from Gemini")
    }
    // Concatenate text parts
    var b strings.Builder
    for _, part := range resp.Candidates[0].Content.Parts {
        if t, ok := part.(genai.Text); ok {
            b.WriteString(string(t))
        }
    }
    text := b.String()

    // Parse JSON or extract JSON like in Claude path
    var out map[string]interface{}
    if err := json.Unmarshal([]byte(text), &out); err == nil {
        return out, nil
    }
    re := regexp.MustCompile(`\{[\s\S]*\}`)
    jsonMatch := re.FindString(text)
    if jsonMatch != "" {
        if err := json.Unmarshal([]byte(jsonMatch), &out); err == nil {
            return out, nil
        }
    }
    return nil, fmt.Errorf("could not parse JSON from Gemini response: %s", text)
}

func ExtractMetadataGemini(ctx context.Context, client *genai.Client, pdfText string) (map[string]interface{}, error) {
    systemPrompt := `You are a research paper analyzer. Extract the title, all authors, and publication date from research papers. Return only valid JSON with no additional text.`
    prompt := fmt.Sprintf(`Extract the title, all authors, and the publication date from the following research paper text. The date might be just a month and year, or more specific. Return only a JSON object with the following structure:
{
  "title": "paper title",
  "authors": ["author1", "author2"],
  "date": "publication date"
}

Text:

%s`, pdfText[:int(math.Min(float64(len(pdfText)), 4000))])
    return CallGemini(ctx, client, prompt, systemPrompt)
}

func GenerateMindmapGemini(ctx context.Context, client *genai.Client, pdfText string) (map[string]interface{}, error) {
    systemPrompt := `You are an expert at creating hierarchical mind maps from academic papers. Create structured JSON mind maps with up to 8 levels of depth. Each node must have: name, tooltip, section, pages, and optionally children. Return only valid JSON with no additional text.`
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
    return CallGemini(ctx, client, prompt, systemPrompt)
}

// UpdateNodeByPath traverses and updates a node based on a path array
func UpdateNodeByPath(root map[string]interface{}, path []interface{}, updates map[string]interface{}) bool {
    var current interface{} = root
    for i, key := range path {
        isLast := i == len(path)-1
        switch idx := key.(type) {
        case string:
            if isLast {
                m, ok := current.(map[string]interface{})
                if !ok {
                    return false
                }
                if _, exists := m[idx]; !exists {
                    return false
                }
                // merge map
                if node, ok := m[idx].(map[string]interface{}); ok {
                    for k, v := range updates {
                        node[k] = v
                    }
                    m[idx] = node
                } else {
                    m[idx] = updates
                }
                return true
            }
            m, ok := current.(map[string]interface{})
            if !ok {
                return false
            }
            current = m[idx]
        case float64: // JSON numbers come as float64
            // next must be array access
            arr, ok := current.([]interface{})
            if !ok {
                return false
            }
            ii := int(idx)
            if ii < 0 || ii >= len(arr) {
                return false
            }
            if isLast {
                node, ok := arr[ii].(map[string]interface{})
                if !ok {
                    return false
                }
                for k, v := range updates {
                    node[k] = v
                }
                arr[ii] = node
                return true
            }
            current = arr[ii]
        default:
            return false
        }
    }
    return false
}

// ---------------------- Action Handlers under /api/mindmaps/{id}/* ---------------------- //

type nodeActionRequest struct {
    NodePath []interface{}          `json:"nodePath"`
    NodeData map[string]interface{} `json:"nodeData"`
}

// RedoDescriptionHandler: POST /api/mindmaps/{id}/redo-description
func RedoDescriptionHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    platform := r.URL.Query().Get("platform")
    if platform == "" { platform = defaultPlatform() }
    id, action := parseMindmapAction(r.URL.Path)
    if id == "" || action != "redo-description" {
        http.NotFound(w, r)
        return
    }

    var req nodeActionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }

    item, err := db.GetMindmapByIDPlatform(r.Context(), platform, id)
    if err != nil || item == nil {
        http.Error(w, "mindmap not found", http.StatusNotFound)
        return
    }
    systemPrompt := "You are an expert at explaining academic concepts. Provide clear, concise explanations in plain English. Return only valid JSON with no additional text."
    prompt := fmt.Sprintf(`Given the full text of a research paper, please rewrite a short, plain-english "tooltip" description for the specific concept: "%s". The description should explain the concept in the context of the paper. Keep it concise.

Return only a JSON object in this format:
{
  "tooltip": "your explanation here"
}

Full Paper Text:
%s`, valueAsString(req.NodeData["name"]), item.PDFText)
    var tooltip string
    switch platform {
    case "aws":
        br, err := NewBedrockClient()
        if err != nil { http.Error(w, "bedrock init error", http.StatusInternalServerError); return }
        result, err := CallClaude(r.Context(), br, prompt, systemPrompt)
        if err != nil { http.Error(w, "LLM error", http.StatusInternalServerError); return }
        tooltip = valueAsString(result["tooltip"])
    case "gcp":
        gm, err := NewGeminiClient(r.Context())
        if err != nil { http.Error(w, "gemini init error", http.StatusInternalServerError); return }
        defer gm.Close()
        result, err := CallGemini(r.Context(), gm, prompt, systemPrompt)
        if err != nil { http.Error(w, "LLM error", http.StatusInternalServerError); return }
        tooltip = valueAsString(result["tooltip"])
    default:
        http.Error(w, "unknown platform", http.StatusBadRequest)
        return
    }

    data := item.MindmapData
    if ok := UpdateNodeByPath(data, req.NodePath, map[string]interface{}{"tooltip": tooltip}); !ok {
        http.Error(w, "node path not found", http.StatusNotFound)
        return
    }
    if err := db.UpdateMindmapPlatform(r.Context(), platform, id, map[string]interface{}{"mindmapData": data, "updatedAt": time.Now().UTC().Format(time.RFC3339)}); err != nil {
        http.Error(w, "update failed", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "newTooltip": tooltip})
}

// RemakeSubtreeHandler: POST /api/mindmaps/{id}/remake-subtree
func RemakeSubtreeHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    platform := r.URL.Query().Get("platform")
    if platform == "" { platform = defaultPlatform() }
    id, action := parseMindmapAction(r.URL.Path)
    if id == "" || action != "remake-subtree" {
        http.NotFound(w, r)
        return
    }
    var req nodeActionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }
    item, err := db.GetMindmapByIDPlatform(r.Context(), platform, id)
    if err != nil || item == nil {
        http.Error(w, "mindmap not found", http.StatusNotFound)
        return
    }
    systemPrompt := "You are an expert at creating hierarchical mind maps from academic papers. Create structured JSON mind maps. Return only valid JSON with no additional text."
    prompt := fmt.Sprintf(`From the research paper provided, expand on the specific topic: "%s". Create a hierarchical list of sub-topics that would fall under this main topic, structured as a mind map.

The root of this new map should be "%s", and it can have children and grandchildren. For each node, provide:
- 'name': topic name
- 'tooltip': plain-english explanation
- 'section': document section
- 'pages': page numbers

Return the response as a JSON object in this format:
{
  "name": "%s",
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

Full Paper Text:
%s`, valueAsString(req.NodeData["name"]), valueAsString(req.NodeData["name"]), valueAsString(req.NodeData["name"]), item.PDFText)

    var newTree map[string]interface{}
    switch platform {
    case "aws":
        br, err := NewBedrockClient()
        if err != nil { http.Error(w, "bedrock init error", http.StatusInternalServerError); return }
        newTree, err = CallClaude(r.Context(), br, prompt, systemPrompt)
        if err != nil { http.Error(w, "LLM error", http.StatusInternalServerError); return }
    case "gcp":
        gm, err := NewGeminiClient(r.Context())
        if err != nil { http.Error(w, "gemini init error", http.StatusInternalServerError); return }
        defer gm.Close()
        newTree, err = CallGemini(r.Context(), gm, prompt, systemPrompt)
        if err != nil { http.Error(w, "LLM error", http.StatusInternalServerError); return }
    default:
        http.Error(w, "unknown platform", http.StatusBadRequest)
        return
    }
    var children []interface{}
    if c, ok := newTree["children"].([]interface{}); ok {
        children = c
    }
    data := item.MindmapData
    if ok := UpdateNodeByPath(data, req.NodePath, map[string]interface{}{"children": children}); !ok {
        http.Error(w, "node path not found", http.StatusNotFound)
        return
    }
    if err := db.UpdateMindmapPlatform(r.Context(), platform, id, map[string]interface{}{"mindmapData": data, "updatedAt": time.Now().UTC().Format(time.RFC3339)}); err != nil {
        http.Error(w, "update failed", http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "newChildren": children})
}

// GoDeeperHandler: POST /api/mindmaps/{id}/go-deeper
func GoDeeperHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    platform := r.URL.Query().Get("platform")
    if platform == "" { platform = "aws" }
    id, action := parseMindmapAction(r.URL.Path)
    if id == "" || action != "go-deeper" {
        http.NotFound(w, r)
        return
    }
    var req nodeActionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }
    item, err := db.GetMindmapByIDPlatform(r.Context(), platform, id)
    if err != nil || item == nil {
        http.Error(w, "mindmap not found", http.StatusNotFound)
        return
    }
    systemPrompt := "You are an expert at expanding academic topics into subtopics. Create structured JSON arrays. Return only valid JSON with no additional text."
    prompt := fmt.Sprintf(`Based on the provided research paper, expand on the topic "%s". Generate a new list of direct sub-topics (children).

For each child, provide:
- 'name': topic name
- 'tooltip': plain-english explanation
- 'section': document section
- 'pages': page numbers

Return this as a JSON object with a single 'children' array:
{
  "children": [
    {
      "name": "subtopic name",
      "tooltip": "explanation",
      "section": "section name",
      "pages": "page numbers"
    }
  ]
}

Full Paper Text:
%s`, valueAsString(req.NodeData["name"]), item.PDFText)

    var result map[string]interface{}
    switch platform {
    case "aws":
        br, err := NewBedrockClient()
        if err != nil { http.Error(w, "bedrock init error", http.StatusInternalServerError); return }
        result, err = CallClaude(r.Context(), br, prompt, systemPrompt)
        if err != nil { http.Error(w, "LLM error", http.StatusInternalServerError); return }
    case "gcp":
        gm, err := NewGeminiClient(r.Context())
        if err != nil { http.Error(w, "gemini init error", http.StatusInternalServerError); return }
        defer gm.Close()
        result, err = CallGemini(r.Context(), gm, prompt, systemPrompt)
        if err != nil { http.Error(w, "LLM error", http.StatusInternalServerError); return }
    default:
        http.Error(w, "unknown platform", http.StatusBadRequest)
        return
    }
    var children []interface{}
    if c, ok := result["children"].([]interface{}); ok {
        children = c
    }
    data := item.MindmapData
    if ok := UpdateNodeByPath(data, req.NodePath, map[string]interface{}{"children": children}); !ok {
        http.Error(w, "node path not found", http.StatusNotFound)
        return
    }
    if err := db.UpdateMindmapPlatform(r.Context(), platform, id, map[string]interface{}{"mindmapData": data, "updatedAt": time.Now().UTC().Format(time.RFC3339)}); err != nil {
        http.Error(w, "update failed", http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "newChildren": children})
}

func parseMindmapAction(path string) (id string, action string) {
    base := strings.TrimPrefix(path, "/api/mindmaps/")
    parts := strings.Split(base, "/")
    if len(parts) == 0 {
        return "", ""
    }
    id = parts[0]
    if len(parts) > 1 {
        action = parts[1]
    }
    return
}

func valueAsString(v interface{}) string {
    if v == nil { return "" }
    if s, ok := v.(string); ok { return s }
    b, _ := json.Marshal(v)
    return string(b)
}

func defaultPlatform() string {
    p := strings.ToLower(strings.TrimSpace(os.Getenv("DEFAULT_PLATFORM")))
    if p == "gcp" { return "gcp" }
    return "aws"
}
