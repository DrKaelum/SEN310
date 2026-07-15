package main

import (
	"context"
	"encoding/base64"
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
	"github.com/google/uuid"
)

const pendingStatus = "pending"

type SignUpRequest struct {
	PerformanceID string `json:"performanceId"`
	PerformerID   string `json:"performerId"`
	CharacterName string `json:"characterName"`
}

type Audition struct {
	ID            string `json:"Id" dynamodbav:"Id"`
	PerformanceID string `json:"performanceId" dynamodbav:"performanceId"`
	PerformerID   string `json:"performerId" dynamodbav:"performerId"`
	CharacterName string `json:"characterName" dynamodbav:"characterName"`
	Status        string `json:"status" dynamodbav:"status"`
}

type SignUpResponse struct {
	Audition
	Message string `json:"message"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type auditionDynamoDBAPI interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

func makeHandler(client auditionDynamoDBAPI, performancesTable string, auditionsTable string, configErr error) func(context.Context, events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return func(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		if event.HTTPMethod == "OPTIONS" {
			return response(200, map[string]string{"message": "CORS preflight OK"})
		}

		body, bodyError := requestBody(event)
		if bodyError != "" {
			return response(400, ErrorResponse{Message: bodyError})
		}
		var request SignUpRequest
		if err := json.Unmarshal([]byte(body), &request); err != nil {
			return response(400, ErrorResponse{Message: "Invalid request body: expected JSON"})
		}
		if strings.TrimSpace(request.PerformanceID) == "" {
			return response(400, ErrorResponse{Message: "Missing required field: performanceId"})
		}
		if strings.TrimSpace(request.PerformerID) == "" {
			return response(400, ErrorResponse{Message: "Missing required field: performerId"})
		}
		if strings.TrimSpace(request.CharacterName) == "" {
			return response(400, ErrorResponse{Message: "Missing required field: characterName"})
		}
		if performancesTable == "" {
			return response(500, ErrorResponse{Message: "Server configuration error: PERFORMANCES_TABLE_NAME is not set"})
		}
		if auditionsTable == "" {
			return response(500, ErrorResponse{Message: "Server configuration error: AUDITIONS_TABLE_NAME is not set"})
		}
		if configErr != nil || client == nil {
			return response(500, ErrorResponse{Message: "Server configuration error: unable to initialize AWS"})
		}

		performance, err := client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName:      aws.String(performancesTable),
			Key:            map[string]types.AttributeValue{"Id": &types.AttributeValueMemberS{Value: request.PerformanceID}},
			ConsistentRead: aws.Bool(true),
		})
		if err != nil {
			return response(500, ErrorResponse{Message: "Failed to verify performance"})
		}
		if len(performance.Item) == 0 {
			return response(404, ErrorResponse{Message: "Performance '" + request.PerformanceID + "' not found"})
		}

		audition := Audition{
			ID:            uuid.NewString(),
			PerformanceID: request.PerformanceID,
			PerformerID:   request.PerformerID,
			CharacterName: request.CharacterName,
			Status:        pendingStatus,
		}
		item, err := attributevalue.MarshalMap(audition)
		if err != nil {
			return response(500, ErrorResponse{Message: "Failed to prepare audition for storage"})
		}
		if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(auditionsTable), Item: item}); err != nil {
			return response(500, ErrorResponse{Message: "Failed to store audition"})
		}

		return response(200, SignUpResponse{Audition: audition, Message: "Audition sign-up created successfully"})
	}
}

func requestBody(event events.APIGatewayProxyRequest) (string, string) {
	body := event.Body
	if event.IsBase64Encoded {
		decodedBody, err := base64.StdEncoding.DecodeString(event.Body)
		if err != nil {
			return "", "Invalid request body: body is not valid base64"
		}
		body = string(decodedBody)
	}
	if strings.TrimSpace(body) == "" {
		return "", "Missing request body"
	}
	return body, ""
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
			"Access-Control-Allow-Methods": "OPTIONS,POST,GET",
		},
		Body: string(bodyJSON),
	}, nil
}

func main() {
	performancesTable := os.Getenv("PERFORMANCES_TABLE_NAME")
	auditionsTable := os.Getenv("AUDITIONS_TABLE_NAME")
	cfg, err := config.LoadDefaultConfig(context.Background())
	var client auditionDynamoDBAPI
	if err == nil {
		client = dynamodb.NewFromConfig(cfg)
	}
	lambda.Start(makeHandler(client, performancesTable, auditionsTable, err))
}
