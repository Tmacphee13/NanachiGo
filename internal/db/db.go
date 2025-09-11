package db

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "strings"
    "sync"

    "github.com/Tmacphee13/NanachiGo/internal/auth"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var (
    tableNameOnce sync.Once
    tableName     string
)

func getTableName() string {
    tableNameOnce.Do(func() {
        v := strings.TrimSpace(os.Getenv("MINDMAPS_TABLE"))
        if v == "" {
            v = "mindmaps"
        }
        tableName = v
    })
    return tableName
}

func GetDynamoDBClient() (*dynamodb.Client, error) {
    // Get AWS config from auth package
    cfg, err := auth.GetAWSConfig()
    if err != nil {
        log.Printf("aws: GetAWSConfig error: %v", err)
        return nil, err
    }
    log.Printf("aws: initializing DynamoDB client (region=%s, table=%s)", cfg.Region, getTableName())
    client := dynamodb.NewFromConfig(cfg)
    return client, nil
}

// PreflightDynamoDB checks connectivity and table existence/permissions.
func PreflightDynamoDB(ctx context.Context) error {
    client, err := GetDynamoDBClient()
    if err != nil {
        return fmt.Errorf("preflight: init dynamodb client: %w", err)
    }
    table := getTableName()
    // Prefer DescribeTable to avoid scanning data
    _, err = client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(table)})
    if err != nil {
        return fmt.Errorf("preflight: describe table %q failed: %w", table, err)
    }
    log.Printf("aws: preflight ok (dynamodb table=%s)", table)
    return nil
}

func ListDynamoDBTables() {

	client, err := GetDynamoDBClient()
	if err != nil {
		log.Fatalf("Error retrieving db client: %v\n", err)
	}

	// create context for the request
	ctx := context.TODO()

	// use client to list tables
	input := &dynamodb.ListTablesInput{}
	output, err := client.ListTables(ctx, input)
	if err != nil {
		log.Fatalf("Failed listing dynamodb tables: %v\n", err)
	}

	// print names of the tables
	fmt.Println("DynamoDB Tables:")
	for _, tableName := range output.TableNames {
		fmt.Println("- ", tableName)
	}

}

/*
Test this function by running the server and curling:
curl http://localhost:3000/api/mindmap
*/
func GetAllMindmaps(w http.ResponseWriter, r *http.Request) {
    platform := r.URL.Query().Get("platform")
    if platform == "" { platform = defaultPlatform() }
    debug := strings.EqualFold(os.Getenv("DEBUG"), "1") || strings.EqualFold(os.Getenv("DEBUG"), "true") || strings.EqualFold(os.Getenv("DEBUG"), "yes")
    if debug {
        log.Printf("api: GET /api/mindmaps platform=%s remote=%s", platform, r.RemoteAddr)
    }

    var resp any
    var err error
    switch platform {
    case "aws":
        // create dynamodb client
        client, e := GetDynamoDBClient()
        if e != nil {
            log.Printf("aws: dynamodb client init failed: %v", e)
            http.Error(w, "Error retrieving db client", http.StatusInternalServerError)
            return
        }
        output, e := client.Scan(r.Context(), &dynamodb.ScanInput{TableName: aws.String(getTableName())})
        if e != nil {
            log.Printf("aws: dynamodb scan failed (table=%s): %v", getTableName(), e)
            http.Error(w, "Error scanning mindmaps table", http.StatusInternalServerError)
            return
        }
        var items []MindmapItem
        for _, it := range output.Items {
            var mm MindmapItem
            if e := attributevalue.UnmarshalMap(it, &mm); e == nil {
                items = append(items, mm)
            }
        }
        if items == nil { items = []MindmapItem{} }
        resp = items
    case "gcp":
        var items []MindmapItem
        items, err = ListMindmapsGCP(r.Context())
        if err != nil {
            log.Printf("gcp: firestore list failed: %v", err)
            // When DEBUG is enabled, surface the underlying error to the client
            if debug {
                http.Error(w, "GCP Firestore list error: "+err.Error(), http.StatusInternalServerError)
            } else {
                http.Error(w, "Error listing firestore mindmaps", http.StatusInternalServerError)
            }
            return
        }
        if items == nil { items = []MindmapItem{} }
        resp = items
    default:
        http.Error(w, "unknown platform", http.StatusBadRequest)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(resp); err != nil {
        http.Error(w, "Error encoding response", http.StatusInternalServerError)
        return
    }
}

// Platform-agnostic wrappers
func CreateMindmapPlatform(ctx context.Context, platform string, item MindmapItem) (string, error) {
    if platform == "gcp" { return CreateMindmapGCP(ctx, item) }
    return CreateMindmap(ctx, item)
}

func GetMindmapByIDPlatform(ctx context.Context, platform, id string) (*MindmapItem, error) {
    if platform == "gcp" { return GetMindmapByIDGCP(ctx, id) }
    return GetMindmapByID(ctx, id)
}

func UpdateMindmapPlatform(ctx context.Context, platform, id string, updates map[string]interface{}) error {
    if platform == "gcp" { return UpdateMindmapGCP(ctx, id, updates) }
    return UpdateMindmap(ctx, id, updates)
}

func DeleteMindmapByIDPlatform(ctx context.Context, platform, id string) (bool, error) {
    if platform == "gcp" { return DeleteMindmapByIDGCP(ctx, id) }
    return DeleteMindmapByID(ctx, id)
}

// DeleteMindmapHandler routes delete to correct backend
func DeleteMindmapHandler(w http.ResponseWriter, r *http.Request) {
    platform := r.URL.Query().Get("platform")
    if platform == "" { platform = defaultPlatform() }
    path := strings.TrimPrefix(r.URL.Path, "/api/mindmaps/")
    parts := strings.Split(path, "/")
    if len(parts) < 1 || parts[0] == "" {
        http.Error(w, "missing id", http.StatusBadRequest)
        return
    }
    id := parts[0]
    deleted, err := DeleteMindmapByIDPlatform(r.Context(), platform, id)
    if err != nil {
        http.Error(w, "error deleting mindmap", http.StatusInternalServerError)
        return
    }
    if !deleted {
        http.Error(w, "mindmap not found", http.StatusNotFound)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    fmt.Fprintf(w, `{"success":true,"message":"Mindmap deleted successfully"}`)
}

func defaultPlatform() string {
    p := strings.ToLower(strings.TrimSpace(os.Getenv("DEFAULT_PLATFORM")))
    if p == "gcp" { return "gcp" }
    return "aws"
}

// ---------------------- Types + CRUD helpers ---------------------- //

type MindmapItem struct {
    ID          string                 `dynamodbav:"id" json:"id"`
    Filename    string                 `dynamodbav:"filename" json:"filename"`
    Title       string                 `dynamodbav:"title" json:"title"`
    Authors     []string               `dynamodbav:"authors" json:"authors"`
    Date        string                 `dynamodbav:"date" json:"date"`
    MindmapData map[string]interface{} `dynamodbav:"mindmapData" json:"mindmapData"`
    PDFText     string                 `dynamodbav:"pdfText" json:"pdfText"`
    CreatedAt   string                 `dynamodbav:"createdAt" json:"createdAt"`
    UpdatedAt   string                 `dynamodbav:"updatedAt" json:"updatedAt"`
}

// CreateMindmap inserts a new item and returns its id
func CreateMindmap(ctx context.Context, item MindmapItem) (string, error) {
    client, err := GetDynamoDBClient()
    if err != nil {
        return "", err
    }
    av, err := attributevalue.MarshalMap(item)
    if err != nil {
        return "", err
    }
    _, err = client.PutItem(ctx, &dynamodb.PutItemInput{
        TableName: aws.String(getTableName()),
        Item:      av,
        ConditionExpression: aws.String("attribute_not_exists(id)"),
    })
    if err != nil {
        return "", err
    }
    return item.ID, nil
}

// GetMindmapByID fetches a single item by id
func GetMindmapByID(ctx context.Context, id string) (*MindmapItem, error) {
    client, err := GetDynamoDBClient()
    if err != nil {
        return nil, err
    }
    out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
        TableName: aws.String(getTableName()),
        Key: map[string]types.AttributeValue{
            "id": &types.AttributeValueMemberS{Value: id},
        },
    })
    if err != nil {
        return nil, err
    }
    if out.Item == nil {
        return nil, nil
    }
    var item MindmapItem
    if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
        return nil, err
    }
    return &item, nil
}

// UpdateMindmap updates arbitrary fields by id
func UpdateMindmap(ctx context.Context, id string, updates map[string]interface{}) error {
    client, err := GetDynamoDBClient()
    if err != nil {
        return err
    }

    // Build UpdateExpression dynamically
    var setExprs []string
    exprAttrNames := map[string]string{}
    exprAttrValues := map[string]types.AttributeValue{}
    i := 0
    for k, v := range updates {
        nameKey := fmt.Sprintf("#n%d", i)
        valueKey := fmt.Sprintf(":v%d", i)
        setExprs = append(setExprs, fmt.Sprintf("%s = %s", nameKey, valueKey))
        exprAttrNames[nameKey] = k
        av, err := attributevalue.Marshal(v)
        if err != nil {
            return err
        }
        exprAttrValues[valueKey] = av
        i++
    }

    _, err = client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
        TableName: aws.String(getTableName()),
        Key: map[string]types.AttributeValue{
            "id": &types.AttributeValueMemberS{Value: id},
        },
        UpdateExpression:          aws.String("SET " + strings.Join(setExprs, ", ")),
        ExpressionAttributeNames:  exprAttrNames,
        ExpressionAttributeValues: exprAttrValues,
    })
    return err
}

// DeleteMindmapByID deletes an item by id, returns true if deleted
func DeleteMindmapByID(ctx context.Context, id string) (bool, error) {
    client, err := GetDynamoDBClient()
    if err != nil {
        return false, err
    }
    _, err = client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
        TableName: aws.String(getTableName()),
        Key: map[string]types.AttributeValue{
            "id": &types.AttributeValueMemberS{Value: id},
        },
    })
    if err != nil {
        return false, err
    }
    return true, nil
}

// ---------------------- HTTP router for /api/mindmaps/* ---------------------- //

// MindmapRouter handles routes like:
// DELETE /api/mindmaps/{id}
// POST   /api/mindmaps/{id}/redo-description
// POST   /api/mindmaps/{id}/remake-subtree
// POST   /api/mindmaps/{id}/go-deeper
func MindmapRouter(w http.ResponseWriter, r *http.Request) {
    path := strings.TrimPrefix(r.URL.Path, "/api/mindmaps/")
    parts := strings.Split(path, "/")
    if len(parts) == 0 || parts[0] == "" {
        http.Error(w, "missing id", http.StatusBadRequest)
        return
    }
    id := parts[0]

    switch r.Method {
    case http.MethodDelete:
        if len(parts) != 1 {
            http.NotFound(w, r)
            return
        }
        deleted, err := DeleteMindmapByID(r.Context(), id)
        if err != nil {
            http.Error(w, "error deleting mindmap", http.StatusInternalServerError)
            return
        }
        if !deleted {
            http.Error(w, "mindmap not found", http.StatusNotFound)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, `{"success":true,"message":"Mindmap deleted successfully"}`)
        return

    case http.MethodPost:
        if len(parts) != 2 {
            http.NotFound(w, r)
            return
        }
        // delegate to utils package which implements these actions
        action := parts[1]
        switch action {
        case "redo-description", "remake-subtree", "go-deeper":
            // to avoid import cycle, call into utils via HTTP from main; we’ll set handlers there.
            http.Error(w, "handler not wired here", http.StatusNotImplemented)
        default:
            http.NotFound(w, r)
        }
        return

    default:
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    }
}
