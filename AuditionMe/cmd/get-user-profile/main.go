package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type User struct {
	ID        string `json:"Id" dynamodbav:"Id"`
	Name      string `json:"name" dynamodbav:"name"`
	Email     string `json:"email" dynamodbav:"email"`
	Phone     string `json:"phone" dynamodbav:"phone"`
	Role      string `json:"role" dynamodbav:"role"`
	CreatedAt string `json:"created_at" dynamodbav:"created_at"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type getItemAPI interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

func makeHandler(client getItemAPI, tableName string, configErr error) func(context.Context, events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return func(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		if event.HTTPMethod == "OPTIONS" {
			return response(200, map[string]string{"message": "CORS preflight OK"})
		}

		userID := strings.TrimSpace(event.PathParameters["userId"])
		if userID == "" {
			return response(400, ErrorResponse{Message: "Missing required path parameter: userId"})
		}
		if tableName == "" {
			return response(500, ErrorResponse{Message: "Server configuration error: TABLE_NAME is not set"})
		}
		if configErr != nil || client == nil {
			return response(500, ErrorResponse{Message: "Server configuration error: unable to initialize AWS"})
		}

		result, err := client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"Id": &types.AttributeValueMemberS{Value: userID},
			},
			ConsistentRead: aws.Bool(true),
		})
		if err != nil {
			return response(500, ErrorResponse{Message: "Failed to read user profile"})
		}
		if len(result.Item) == 0 {
			return response(404, ErrorResponse{Message: "User '" + userID + "' not found"})
		}

		var user User
		if err := attributevalue.UnmarshalMap(result.Item, &user); err != nil {
			return response(500, ErrorResponse{Message: "Failed to read stored user profile"})
		}
		return response(200, user)
	}
}

func response(statusCode int, body any) (events.APIGatewayProxyResponse, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return events.APIGatewayProxyResponse{}, err
	}
	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Headers": "Content-Type,Authorization",
			"Access-Control-Allow-Methods": "OPTIONS,POST,GET,DELETE",
		},
		Body: string(bodyJSON),
	}, nil
}

func main() {
	tableName := os.Getenv("TABLE_NAME")
	cfg, err := config.LoadDefaultConfig(context.Background())
	var client getItemAPI
	if err == nil {
		client = newDynamoDBClient(cfg)
	}
	lambda.Start(makeHandler(client, tableName, err))
}

func newDynamoDBClient(cfg aws.Config) *dynamodb.Client {
	endpoint := strings.TrimSpace(os.Getenv("DYNAMODB_ENDPOINT"))
	if endpoint == "" {
		return dynamodb.NewFromConfig(cfg)
	}
	return dynamodb.NewFromConfig(cfg, func(options *dynamodb.Options) {
		options.BaseEndpoint = aws.String(endpoint)
	})
}
