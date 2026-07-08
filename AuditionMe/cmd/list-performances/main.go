package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type Performance struct {
	Title            string   `json:"title"`
	Director         string   `json:"director"`
	Venue            string   `json:"venue"`
	IsLive           bool     `json:"isLive"`
	PerformanceDates []string `json:"performanceDates"`
}

func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if event.HTTPMethod == "OPTIONS" {
		return response(200, map[string]string{"message": "CORS preflight OK"})
	}

	performances := []Performance{
		{
			Title:            "The Glass Menagerie",
			Director:         "Morgan Lee",
			Venue:            "AuditionMe Black Box Theatre",
			IsLive:           true,
			PerformanceDates: []string{"2026-08-14", "2026-08-15", "2026-08-16"},
		},
		{
			Title:            "A Midsummer Night's Dream",
			Director:         "Jordan Rivera",
			Venue:            "AuditionMe Outdoor Stage",
			IsLive:           true,
			PerformanceDates: []string{"2026-09-04", "2026-09-05"},
		},
		{
			Title:            "Songs for a New World",
			Director:         "Taylor Chen",
			Venue:            "AuditionMe Studio A",
			IsLive:           false,
			PerformanceDates: []string{"2026-10-10", "2026-10-11", "2026-10-12"},
		},
	}

	return response(200, map[string][]Performance{"performances": performances})
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
	result, err := handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod: "GET",
		Path:       "/api/performances",
	})

	fmt.Println("=== list performances ===")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	var body any
	if json.Unmarshal([]byte(result.Body), &body) != nil {
		body = result.Body
	}

	fmt.Printf("Status: %d\n", result.StatusCode)
	fmt.Printf("Headers: %v\n", result.Headers)
	fmt.Printf("Body: %v\n", body)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "test" {
		runLocalTests()
		return
	}

	lambda.Start(handler)
}
