package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"
)

type CreateUserRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

type User struct {
	ID        string `json:"Id" dynamodbav:"Id"`
	Name      string `json:"name" dynamodbav:"name"`
	Email     string `json:"email" dynamodbav:"email"`
	Phone     string `json:"phone" dynamodbav:"phone"`
	Role      string `json:"role" dynamodbav:"role"`
	CreatedAt string `json:"created_at" dynamodbav:"created_at"`
}

type CreateUserResponse struct {
	User
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

		var request CreateUserRequest
		body, bodyError := requestBody(event)
		if bodyError != "" {
			return response(400, ErrorResponse{Message: bodyError})
		}
		if err := json.Unmarshal([]byte(body), &request); err != nil {
			return response(400, ErrorResponse{Message: "Invalid request body: expected JSON"})
		}

		if request.Name == "" {
			return response(400, ErrorResponse{Message: "Missing required field: name"})
		}
		if request.Email == "" {
			return response(400, ErrorResponse{Message: "Missing required field: email"})
		}
		if request.Phone == "" {
			return response(400, ErrorResponse{Message: "Missing required field: phone"})
		}
		if request.Role == "" {
			return response(400, ErrorResponse{Message: "Missing required field: role"})
		}
		if request.Password == "" {
			return response(400, ErrorResponse{Message: "Missing required field: password"})
		}
		if !validateEmail(request.Email) {
			return response(400, ErrorResponse{Message: "Invalid email: email must include @, a non-empty local part, a domain dot, and a non-empty TLD"})
		}
		if request.Role != "performer" && request.Role != "director" {
			return response(400, ErrorResponse{Message: "Invalid role: role must be exactly performer or director"})
		}

		user := User{
			ID:        uuid.NewString(),
			Name:      request.Name,
			Email:     request.Email,
			Phone:     request.Phone,
			Role:      request.Role,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		item, err := attributevalue.MarshalMap(user)
		if err != nil {
			return response(500, ErrorResponse{Message: "Failed to prepare user for storage"})
		}
		if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(tableName),
			Item:      item,
		}); err != nil {
			return response(500, ErrorResponse{Message: "Failed to store user"})
		}

		return response(200, CreateUserResponse{User: user, Message: "User created successfully"})
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

func validateEmail(email string) bool {
	emailPattern := regexp.MustCompile(`^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$`)
	return emailPattern.MatchString(email)
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
