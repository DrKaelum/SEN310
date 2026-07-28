package main

import (
	"context"
	"encoding/json"
	"log/slog"
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

const unavailablePerformanceTitle = "Performance unavailable"

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

type Audition struct {
	ID               string `json:"Id" dynamodbav:"Id"`
	PerformanceID    string `json:"performanceId" dynamodbav:"performanceId"`
	PerformerID      string `json:"performerId" dynamodbav:"performerId"`
	CharacterName    string `json:"characterName" dynamodbav:"characterName"`
	Status           string `json:"status" dynamodbav:"status"`
	Notified         bool   `json:"notified" dynamodbav:"notified"`
	PerformanceTitle string `json:"performanceTitle" dynamodbav:"-"`
}

type PerformanceTitle struct {
	Title string `dynamodbav:"title"`
}

type GetUserAuditionsResponse struct {
	Auditions []Audition `json:"auditions"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type auditionHistoryDynamoDBAPI interface {
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

func makeHandler(client auditionHistoryDynamoDBAPI, auditionsTable string, performancesTable string, configErr error) func(context.Context, events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return func(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		if event.HTTPMethod == "OPTIONS" {
			return response(200, map[string]string{"message": "CORS preflight OK"})
		}

		userID := strings.TrimSpace(event.PathParameters["userId"])
		if userID == "" {
			return response(400, ErrorResponse{Message: "Missing required path parameter: userId"})
		}
		if auditionsTable == "" || performancesTable == "" {
			return response(500, ErrorResponse{Message: "Server configuration error: table name is not set"})
		}
		if configErr != nil || client == nil {
			return response(500, ErrorResponse{Message: "Server configuration error: unable to initialize AWS"})
		}

		input := &dynamodb.ScanInput{
			TableName:        aws.String(auditionsTable),
			FilterExpression: aws.String("#performerId = :userId"),
			ExpressionAttributeNames: map[string]string{
				"#performerId": "performerId",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":userId": &types.AttributeValueMemberS{Value: userID},
			},
		}

		auditions := make([]Audition, 0)
		for {
			result, err := client.Scan(ctx, input)
			if err != nil {
				return response(500, ErrorResponse{Message: "Failed to read user auditions"})
			}
			var page []Audition
			if err := attributevalue.UnmarshalListOfMaps(result.Items, &page); err != nil {
				return response(500, ErrorResponse{Message: "Failed to decode user auditions"})
			}
			auditions = append(auditions, page...)
			if len(result.LastEvaluatedKey) == 0 {
				break
			}
			input.ExclusiveStartKey = result.LastEvaluatedKey
		}

		for index := range auditions {
			performance, err := client.GetItem(ctx, &dynamodb.GetItemInput{
				TableName: aws.String(performancesTable),
				Key: map[string]types.AttributeValue{
					"Id": &types.AttributeValueMemberS{Value: auditions[index].PerformanceID},
				},
				ConsistentRead: aws.Bool(true),
			})
			if err != nil {
				return response(500, ErrorResponse{Message: "Failed to enrich audition history"})
			}
			if len(performance.Item) == 0 {
				auditions[index].PerformanceTitle = unavailablePerformanceTitle
				logger.WarnContext(ctx, "referenced performance is unavailable",
					"action", "get_user_auditions",
					"outcome", "performance_unavailable",
					"auditionId", auditions[index].ID,
					"performanceId", auditions[index].PerformanceID,
				)
				continue
			}
			var title PerformanceTitle
			if err := attributevalue.UnmarshalMap(performance.Item, &title); err != nil {
				return response(500, ErrorResponse{Message: "Failed to decode performance title"})
			}
			auditions[index].PerformanceTitle = title.Title
		}

		return response(200, GetUserAuditionsResponse{Auditions: auditions})
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
	auditionsTable := os.Getenv("AUDITIONS_TABLE_NAME")
	performancesTable := os.Getenv("PERFORMANCES_TABLE_NAME")
	cfg, err := config.LoadDefaultConfig(context.Background())
	var client auditionHistoryDynamoDBAPI
	if err == nil {
		client = newDynamoDBClient(cfg)
	}
	lambda.Start(makeHandler(client, auditionsTable, performancesTable, err))
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
