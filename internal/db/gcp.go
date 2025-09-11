package db

import (
    "encoding/json"
    "context"
    "fmt"
    "log"
    "os"
    "time"

    "cloud.google.com/go/firestore"
    "github.com/google/uuid"
    "google.golang.org/api/iterator"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

const FS_COLLECTION string = "mindmaps"

func getFirestoreClient(ctx context.Context) (*firestore.Client, string, error) {
    projectID := os.Getenv("GCP_PROJECT_ID")
    if projectID == "" {
        log.Printf("gcp: GCP_PROJECT_ID not set")
        return nil, "", fmt.Errorf("GCP_PROJECT_ID not set")
    }
    adc := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
    if adc == "" {
        log.Printf("gcp: GOOGLE_APPLICATION_CREDENTIALS not set; relying on Application Default Credentials")
    } else {
        if _, err := os.Stat(adc); err != nil {
            log.Printf("gcp: GOOGLE_APPLICATION_CREDENTIALS points to missing file: %s (%v)", adc, err)
        } else {
            log.Printf("gcp: using GOOGLE_APPLICATION_CREDENTIALS file: %s", adc)
        }
    }
    client, err := firestore.NewClient(ctx, projectID)
    if err != nil {
        log.Printf("gcp: failed to init firestore client (project=%s): %v", projectID, err)
        return nil, "", err
    }
    return client, projectID, nil
}

// PreflightFirestore verifies credentials/project by attempting a harmless read
// against the default collection. NotFound is considered success (access ok).
func PreflightFirestore(ctx context.Context) error {
    client, project, err := getFirestoreClient(ctx)
    if err != nil {
        return fmt.Errorf("preflight: init firestore client: %w", err)
    }
    defer client.Close()
    _, err = client.Collection(FS_COLLECTION).Doc("_preflight_").Get(ctx)
    if err != nil {
        if status.Code(err) == codes.NotFound {
            log.Printf("gcp: preflight ok (project=%s, collection=%s)", project, FS_COLLECTION)
            return nil
        }
        return fmt.Errorf("preflight: firestore doc get failed: %w", err)
    }
    log.Printf("gcp: preflight ok (project=%s, collection=%s, doc exists)", project, FS_COLLECTION)
    return nil
}

// ---------------- Firestore CRUD (GCP) ---------------- //

func CreateMindmapGCP(ctx context.Context, item MindmapItem) (string, error) {
    if item.ID == "" {
        item.ID = uuid.New().String()
    }
    client, _, err := getFirestoreClient(ctx)
    if err != nil {
        return "", err
    }
    defer client.Close()

    _, err = client.Collection(FS_COLLECTION).Doc(item.ID).Set(ctx, item)
    if err != nil {
        return "", err
    }
    return item.ID, nil
}

func GetMindmapByIDGCP(ctx context.Context, id string) (*MindmapItem, error) {
    client, _, err := getFirestoreClient(ctx)
    if err != nil {
        return nil, err
    }
    defer client.Close()

    snap, err := client.Collection(FS_COLLECTION).Doc(id).Get(ctx)
    if err != nil {
        if status.Code(err) == codes.NotFound {
            return nil, nil
        }
        return nil, err
    }
    item := snapshotToMindmapItem(snap)
    return &item, nil
}

func UpdateMindmapGCP(ctx context.Context, id string, updates map[string]interface{}) error {
    client, _, err := getFirestoreClient(ctx)
    if err != nil {
        return err
    }
    defer client.Close()
    _, err = client.Collection(FS_COLLECTION).Doc(id).Set(ctx, updates, firestore.MergeAll)
    return err
}

func DeleteMindmapByIDGCP(ctx context.Context, id string) (bool, error) {
    client, _, err := getFirestoreClient(ctx)
    if err != nil {
        return false, err
    }
    defer client.Close()
    _, err = client.Collection(FS_COLLECTION).Doc(id).Delete(ctx)
    if err != nil {
        return false, err
    }
    return true, nil
}

func ListMindmapsGCP(ctx context.Context) ([]MindmapItem, error) {
    client, _, err := getFirestoreClient(ctx)
    if err != nil {
        return nil, err
    }
    defer client.Close()

    it := client.Collection(FS_COLLECTION).Documents(ctx)
    defer it.Stop()

    var res []MindmapItem
    for {
        doc, err := it.Next()
        if err == iterator.Done {
            break
        }
        if err != nil {
            // Log the underlying gRPC status code to help diagnose IAM/rules issues.
            log.Printf("gcp: firestore list iterator error: code=%s err=%v", status.Code(err), err)
            return res, fmt.Errorf("firestore list failed: %w", err)
        }
        item := snapshotToMindmapItem(doc)
        res = append(res, item)
    }
    if res == nil { res = []MindmapItem{} }
    return res, nil
}

// snapshotToMindmapItem converts a Firestore document snapshot into a MindmapItem
// tolerant of differing field types (e.g., Timestamp vs string).
func snapshotToMindmapItem(snap *firestore.DocumentSnapshot) MindmapItem {
    data := snap.Data()

    // Helper to fetch the first present value among several candidate keys
    val := func(keys ...string) any {
        for _, k := range keys {
            if v, ok := data[k]; ok {
                if v != nil {
                    return v
                }
            }
        }
        return nil
    }

    getString := func(keys ...string) string {
        v := val(keys...)
        if v == nil { return "" }
        switch t := v.(type) {
        case string:
            return t
        case fmt.Stringer:
            return t.String()
        default:
            return fmt.Sprintf("%v", t)
        }
    }

    toISOString := func(v any) string {
        switch t := v.(type) {
        case string:
            return t
        case time.Time:
            return t.UTC().Format(time.RFC3339)
        case *time.Time:
            if t == nil { return "" }
            return t.UTC().Format(time.RFC3339)
        case map[string]any:
            // Handle {_seconds: #} shape if present
            if s, ok := t["_seconds"]; ok {
                switch sv := s.(type) {
                case int64:
                    return time.Unix(sv, 0).UTC().Format(time.RFC3339)
                case float64:
                    return time.Unix(int64(sv), 0).UTC().Format(time.RFC3339)
                }
            }
            if s, ok := t["seconds"]; ok {
                switch sv := s.(type) {
                case int64:
                    return time.Unix(sv, 0).UTC().Format(time.RFC3339)
                case float64:
                    return time.Unix(int64(sv), 0).UTC().Format(time.RFC3339)
                }
            }
            return ""
        default:
            return ""
        }
    }

    toStringSlice := func(v any) []string {
        if v == nil { return []string{} }
        switch arr := v.(type) {
        case []string:
            return arr
        case []any:
            out := make([]string, 0, len(arr))
            for _, e := range arr {
                switch s := e.(type) {
                case string:
                    out = append(out, s)
                default:
                    out = append(out, fmt.Sprintf("%v", s))
                }
            }
            return out
        case string:
            return []string{arr}
        default:
            return []string{}
        }
    }

    item := MindmapItem{
        ID:          snap.Ref.ID,
        Filename:    getString("filename", "Filename"),
        Title:       getString("title", "Title"),
        Authors:     toStringSlice(val("authors", "Authors")),
        Date:        getString("date", "Date"),
        PDFText:     getString("pdfText", "PDFText"),
        CreatedAt:   toISOString(val("createdAt", "CreatedAt")),
        UpdatedAt:   toISOString(val("updatedAt", "UpdatedAt")),
        MindmapData: nil,
    }

    // Mindmap data may be stored under either casing depending on writer
    if mdv := val("mindmapData", "MindmapData"); mdv != nil {
        switch md := mdv.(type) {
        case map[string]interface{}:
            item.MindmapData = md
        case string:
            // Try to parse JSON string if stored as string
            var m map[string]interface{}
            if err := json.Unmarshal([]byte(md), &m); err == nil {
                item.MindmapData = m
            }
        case []byte:
            var m map[string]interface{}
            if err := json.Unmarshal(md, &m); err == nil {
                item.MindmapData = m
            }
        }
    }

    return item
}
