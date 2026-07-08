package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/google/uuid"
)

type CreateUserEvent struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

type LambdaResponse struct {
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
}

type SuccessBody struct {
	ID        string `json:"Id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	Message   string `json:"message"`
}

type ErrorBody struct {
	Message string `json:"message"`
}

func handler(ctx context.Context, event CreateUserEvent) (LambdaResponse, error) {
	if event.Name == "" {
		return makeResponse(400, ErrorBody{Message: "Missing required field: name"})
	}

	if event.Email == "" {
		return makeResponse(400, ErrorBody{Message: "Missing required field: email"})
	}

	if event.Phone == "" {
		return makeResponse(400, ErrorBody{Message: "Missing required field: phone"})
	}

	if event.Role == "" {
		return makeResponse(400, ErrorBody{Message: "Missing required field: role"})
	}

	if event.Password == "" {
		return makeResponse(400, ErrorBody{Message: "Missing required field: password"})
	}

	if !validateEmail(event.Email) {
		return makeResponse(400, ErrorBody{Message: "Invalid email: email must include @, a non-empty local part, a domain dot, and a non-empty TLD"})
	}

	if event.Role != "performer" && event.Role != "director" {
		return makeResponse(400, ErrorBody{Message: "Invalid role: role must be exactly performer or director"})
	}

	body := SuccessBody{
		ID:        uuid.NewString(),
		Name:      event.Name,
		Email:     event.Email,
		Phone:     event.Phone,
		Role:      event.Role,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Message:   "User created successfully",
	}

	return makeResponse(200, body)
}

func validateEmail(email string) bool {
	emailPattern := regexp.MustCompile(`^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$`)
	return emailPattern.MatchString(email)
}

func makeResponse(statusCode int, body any) (LambdaResponse, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return LambdaResponse{}, err
	}

	return LambdaResponse{
		StatusCode: statusCode,
		Body:       string(bodyJSON),
	}, nil
}

func runLocalTests() {
	tests := []CreateUserEvent{
		{
			Name:     "Avery Stone",
			Email:    "avery@example.com",
			Phone:    "555-0101",
			Role:     "performer",
			Password: "performer-password",
		},
		{
			Name:     "Morgan Lee",
			Email:    "morgan@example.org",
			Phone:    "555-0102",
			Role:     "director",
			Password: "director-password",
		},
		{
			Name:     "No Phone",
			Email:    "nophone@example.com",
			Role:     "performer",
			Password: "password",
		},
		{
			Name:     "Invalid Role",
			Email:    "role@example.com",
			Phone:    "555-0104",
			Role:     "producer",
			Password: "password",
		},
		{
			Name:     "No At Symbol",
			Email:    "no-at-symbol.example.com",
			Phone:    "555-0105",
			Role:     "performer",
			Password: "password",
		},
		{
			Name:     "No Domain Dot",
			Email:    "nodot@example",
			Phone:    "555-0106",
			Role:     "director",
			Password: "password",
		},
		{
			Name:     "No Password",
			Email:    "nopassword@example.com",
			Phone:    "555-0107",
			Role:     "performer",
			Password: "",
		},
	}

	for testNumber, event := range tests {
		result, err := handler(context.Background(), event)
		if err != nil {
			fmt.Printf("Test %d error: %v\n\n", testNumber+1, err)
			continue
		}

		var body any
		err = json.Unmarshal([]byte(result.Body), &body)
		if err != nil {
			fmt.Printf("Test %d body parse error: %v\n\n", testNumber+1, err)
			continue
		}

		fmt.Printf("Test %d\n", testNumber+1)
		fmt.Printf("Input:  %+v\n", event)
		fmt.Printf("Status: %d\n", result.StatusCode)
		fmt.Printf("Body:   %v\n\n", body)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "test" {
		runLocalTests()
		return
	}

	lambda.Start(handler)
}
