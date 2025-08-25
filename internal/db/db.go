package db

import (
	"context"
	"fmt"
	"log"

	"github.com/Tmacphee13/NanachiGo/internal/auth"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

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
