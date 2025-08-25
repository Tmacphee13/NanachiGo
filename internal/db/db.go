package db

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/Tmacphee13/NanachiGo/internal/auth"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

const TABLE_NAME string = "mindmaps"

func GetDynamoDBClient() (*dynamodb.Client, error) {
	// Get AWS config from auth package
	cfg, err := auth.GetAWSConfig()
	if err != nil {
		return nil, err
	}
	client := dynamodb.NewFromConfig(cfg)
	return client, nil
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

	// create dynamodb client
	db_client, err := GetDynamoDBClient()
	if err != nil {
		http.Error(w, "Error retrieving db client", http.StatusInternalServerError)
		return
	}

	// This does not paginate, we will need to update if planning on expanding beyond 1MB of data
	// https://aws.github.io/aws-sdk-go-v2/docs/services/dynamodb/pagination/
	output, err := db_client.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: aws.String(TABLE_NAME),
	})
	if err != nil {
		http.Error(w, "Error scanning mindmaps table", http.StatusInternalServerError)
		return
	}

	// Print the items
	for _, item := range output.Items {
		fmt.Println(item)
	}

	// Unmarshal the items into Go maps
	var results []map[string]interface{}
	for _, item := range output.Items {
		var result map[string]interface{}
		err = attributevalue.UnmarshalMap(item, &result)
		if err != nil {
			http.Error(w, "Error unmarshalling item", http.StatusInternalServerError)
			return
		}
		results = append(results, result)
	}

	// Encode the results into JSON and write to HTTP response
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(results)
	if err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
		return
	}

}
