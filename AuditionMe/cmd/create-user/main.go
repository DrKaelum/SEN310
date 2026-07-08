package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/google/uuid"
)

type CreateUserRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

type CreateUserResponse struct {
	ID        string `json:"Id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	Message   string `json:"message"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if event.HTTPMethod == "OPTIONS" {
		return response(200, map[string]string{"message": "CORS preflight OK"})
	}

	var request CreateUserRequest
	body := event.Body
	if event.IsBase64Encoded {
		decodedBody, err := base64.StdEncoding.DecodeString(event.Body)
		if err != nil {
			return response(400, ErrorResponse{Message: "Invalid request body: body is not valid base64"})
		}
		body = string(decodedBody)
	}

	if body == "" {
		return response(400, ErrorResponse{Message: "Missing request body"})
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

	bodyResponse := CreateUserResponse{
		ID:        uuid.NewString(),
		Name:      request.Name,
		Email:     request.Email,
		Phone:     request.Phone,
		Role:      request.Role,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Message:   "User created successfully",
	}

	return response(200, bodyResponse)
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

func runLocalTests() {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "valid request",
			body: `{"name":"Avery Stone","email":"avery@example.com","phone":"555-0101","role":"performer","password":"performer-password"}`,
		},
		{
			name: "missing field",
			body: `{"name":"No Phone","email":"nophone@example.com","role":"performer","password":"password"}`,
		},
		{
			name: "invalid email",
			body: `{"name":"Invalid Email","email":"invalid-email","phone":"555-0103","role":"director","password":"password"}`,
		},
		{
			name: "OPTIONS preflight",
			body: "",
		},
	}

	for _, test := range tests {
		method := "POST"
		if test.name == "OPTIONS preflight" {
			method = "OPTIONS"
		}

		result, err := handler(context.Background(), events.APIGatewayProxyRequest{
			HTTPMethod: method,
			Path:       "/api/users",
			Body:       test.body,
		})
		printResult(test.name, result, err)
	}
}

func printResult(name string, result events.APIGatewayProxyResponse, err error) {
	fmt.Printf("=== %s ===\n", name)
	if err != nil {
		fmt.Printf("Error: %v\n\n", err)
		return
	}

	var body any
	if json.Unmarshal([]byte(result.Body), &body) != nil {
		body = result.Body
	}

	fmt.Printf("Status: %d\n", result.StatusCode)
	fmt.Printf("Headers: %v\n", result.Headers)
	fmt.Printf("Body: %v\n\n", body)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "test" {
		runLocalTests()
		return
	}

	lambda.Start(handler)
}
