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
	"github.com/google/uuid"
)

type PostPerformanceRequest struct {
	Title            string   `json:"title"`
	Director         string   `json:"director"`
	CastingDirector  string   `json:"castingDirector"`
	Venue            string   `json:"venue"`
	PerformanceDates []string `json:"performanceDates"`
	Characters       []string `json:"characters"`
	IsLive           *bool    `json:"isLive"`
}

type Performance struct {
	ID               string   `json:"Id" dynamodbav:"Id"`
	Title            string   `json:"title" dynamodbav:"title"`
	Director         string   `json:"director" dynamodbav:"director"`
	CastingDirector  string   `json:"castingDirector" dynamodbav:"castingDirector"`
	Venue            string   `json:"venue" dynamodbav:"venue"`
	PerformanceDates []string `json:"performanceDates" dynamodbav:"performanceDates"`
	Characters       []string `json:"characters" dynamodbav:"characters"`
	IsLive           bool     `json:"isLive" dynamodbav:"isLive"`
}

type PostPerformanceResponse struct {
	Performance
	Message string `json:"message"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type putItemAPI interface {
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

func makeHandler(client putItemAPI, tableName string, configErr error) func(context.Context, events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return func(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		if event.HTTPMethod == "OPTIONS" {
			return response(200, map[string]string{"message": "CORS preflight OK"})
		}
		if tableName == "" {
			return response(500, ErrorResponse{Message: "Server configuration error: TABLE_NAME is not set"})
		}
		if configErr != nil || client == nil {
			return response(500, ErrorResponse{Message: "Server configuration error: unable to initialize AWS"})
		}

		body, bodyError := requestBody(event)
		if bodyError != "" {
			return response(400, ErrorResponse{Message: bodyError})
		}
		var request PostPerformanceRequest
		if err := json.Unmarshal([]byte(body), &request); err != nil {
			return response(400, ErrorResponse{Message: "Invalid request body: expected JSON"})
		}
		if message := validateRequest(request); message != "" {
			return response(400, ErrorResponse{Message: message})
		}

		performance := Performance{
			ID:               uuid.NewString(),
			Title:            request.Title,
			Director:         request.Director,
			CastingDirector:  request.CastingDirector,
			Venue:            request.Venue,
			PerformanceDates: request.PerformanceDates,
			Characters:       request.Characters,
			IsLive:           *request.IsLive,
		}
		item, err := attributevalue.MarshalMap(performance)
		if err != nil {
			return response(500, ErrorResponse{Message: "Failed to prepare performance for storage"})
		}
		if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(tableName), Item: item}); err != nil {
			return response(500, ErrorResponse{Message: "Failed to store performance"})
		}

		return response(200, PostPerformanceResponse{Performance: performance, Message: "Performance created successfully"})
	}
}

func validateRequest(request PostPerformanceRequest) string {
	requiredStrings := []struct {
		name  string
		value string
	}{
		{"title", request.Title},
		{"director", request.Director},
		{"castingDirector", request.CastingDirector},
		{"venue", request.Venue},
	}
	for _, field := range requiredStrings {
		if strings.TrimSpace(field.value) == "" {
			return "Missing required field: " + field.name
		}
	}
	if len(request.PerformanceDates) == 0 {
		return "performanceDates must be a non-empty list"
	}
	for _, date := range request.PerformanceDates {
		if strings.TrimSpace(date) == "" {
			return "performanceDates must not contain empty values"
		}
	}
	if len(request.Characters) == 0 {
		return "characters must be a non-empty list"
	}
	for _, character := range request.Characters {
		if strings.TrimSpace(character) == "" {
			return "characters must not contain empty values"
		}
	}
	if request.IsLive == nil {
		return "Missing required field: isLive"
	}
	return ""
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
	tableName := os.Getenv("TABLE_NAME")
	cfg, err := config.LoadDefaultConfig(context.Background())
	var client putItemAPI
	if err == nil {
		client = dynamodb.NewFromConfig(cfg)
	}
	lambda.Start(makeHandler(client, tableName, err))
}
