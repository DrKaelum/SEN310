package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type DeletePerformanceResponse struct {
	Message       string `json:"message"`
	PerformanceID string `json:"performanceId"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type deleteItemAPI interface {
	DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

func makeHandler(client deleteItemAPI, tableName string, configErr error) func(context.Context, events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return func(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		if event.HTTPMethod == "OPTIONS" {
			return response(200, map[string]string{"message": "CORS preflight OK"})
		}

		performanceID := strings.TrimSpace(event.PathParameters["performanceId"])
		if performanceID == "" {
			return response(400, ErrorResponse{Message: "Missing required path parameter: performanceId"})
		}
		if tableName == "" {
			return response(500, ErrorResponse{Message: "Server configuration error: TABLE_NAME is not set"})
		}
		if configErr != nil || client == nil {
			return response(500, ErrorResponse{Message: "Server configuration error: unable to initialize AWS"})
		}

		result, err := client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"Id": &types.AttributeValueMemberS{Value: performanceID},
			},
			ConditionExpression: aws.String("attribute_exists(Id)"),
			ReturnValues:        types.ReturnValueAllOld,
		})
		if err != nil {
			var conditionFailed *types.ConditionalCheckFailedException
			if errors.As(err, &conditionFailed) {
				return response(404, ErrorResponse{Message: "Performance '" + performanceID + "' not found"})
			}
			return response(500, ErrorResponse{Message: "Failed to delete performance"})
		}
		if len(result.Attributes) == 0 {
			return response(404, ErrorResponse{Message: "Performance '" + performanceID + "' not found"})
		}

		return response(200, DeletePerformanceResponse{
			Message:       "Performance deleted successfully",
			PerformanceID: performanceID,
		})
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
	var client deleteItemAPI
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
