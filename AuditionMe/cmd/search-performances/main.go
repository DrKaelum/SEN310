package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

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

type SearchPerformancesResponse struct {
	Performances []Performance `json:"performances"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type scanAPI interface {
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
}

func makeHandler(client scanAPI, tableName string, configErr error) func(context.Context, events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
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

		input := &dynamodb.ScanInput{TableName: aws.String(tableName)}
		if event.QueryStringParameters["live"] == "true" {
			input.FilterExpression = aws.String("#live = :true")
			input.ExpressionAttributeNames = map[string]string{"#live": "isLive"}
			input.ExpressionAttributeValues = map[string]types.AttributeValue{
				":true": &types.AttributeValueMemberBOOL{Value: true},
			}
		}

		performances := make([]Performance, 0)
		for {
			result, err := client.Scan(ctx, input)
			if err != nil {
				return response(500, ErrorResponse{Message: "Failed to search performances"})
			}
			var page []Performance
			if err := attributevalue.UnmarshalListOfMaps(result.Items, &page); err != nil {
				return response(500, ErrorResponse{Message: "Failed to read stored performances"})
			}
			performances = append(performances, page...)
			if len(result.LastEvaluatedKey) == 0 {
				break
			}
			input.ExclusiveStartKey = result.LastEvaluatedKey
		}

		return response(200, SearchPerformancesResponse{Performances: performances})
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
			"Access-Control-Allow-Methods": "OPTIONS,POST,GET",
		},
		Body: string(bodyJSON),
	}, nil
}

func main() {
	tableName := os.Getenv("TABLE_NAME")
	cfg, err := config.LoadDefaultConfig(context.Background())
	var client scanAPI
	if err == nil {
		client = dynamodb.NewFromConfig(cfg)
	}
	lambda.Start(makeHandler(client, tableName, err))
}
