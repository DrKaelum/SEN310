package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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

const (
	pendingStatus = "pending"
	castStatus    = "cast"
)

type CastPerformerRequest struct {
	AuditionID string `json:"auditionId"`
}

type Audition struct {
	ID            string `json:"Id" dynamodbav:"Id"`
	PerformanceID string `json:"performanceId" dynamodbav:"performanceId"`
	PerformerID   string `json:"performerId" dynamodbav:"performerId"`
	CharacterName string `json:"characterName" dynamodbav:"characterName"`
	Status        string `json:"status" dynamodbav:"status"`
}

type CastPerformerResponse struct {
	Audition
	Message string `json:"message"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type castDynamoDBAPI interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

func makeHandler(client castDynamoDBAPI, auditionsTable string, configErr error) func(context.Context, events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return func(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		if event.HTTPMethod == "OPTIONS" {
			return response(200, map[string]string{"message": "CORS preflight OK"})
		}

		performanceID := strings.TrimSpace(event.PathParameters["performanceId"])
		if performanceID == "" {
			return response(400, ErrorResponse{Message: "Missing required path parameter: performanceId"})
		}

		body, bodyError := requestBody(event)
		if bodyError != "" {
			return response(400, ErrorResponse{Message: bodyError})
		}
		var request CastPerformerRequest
		if err := json.Unmarshal([]byte(body), &request); err != nil {
			return response(400, ErrorResponse{Message: "Invalid request body: expected JSON"})
		}
		request.AuditionID = strings.TrimSpace(request.AuditionID)
		if request.AuditionID == "" {
			return response(400, ErrorResponse{Message: "Missing required field: auditionId"})
		}

		if auditionsTable == "" {
			return response(500, ErrorResponse{Message: "Server configuration error: AUDITIONS_TABLE_NAME is not set"})
		}
		if configErr != nil || client == nil {
			return response(500, ErrorResponse{Message: "Server configuration error: unable to initialize AWS"})
		}

		key := map[string]types.AttributeValue{
			"Id": &types.AttributeValueMemberS{Value: request.AuditionID},
		}
		result, err := client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName:      aws.String(auditionsTable),
			Key:            key,
			ConsistentRead: aws.Bool(true),
		})
		if err != nil {
			return response(500, ErrorResponse{Message: "Failed to read audition"})
		}
		if len(result.Item) == 0 {
			return response(404, ErrorResponse{Message: "Audition '" + request.AuditionID + "' not found"})
		}

		var audition Audition
		if err := attributevalue.UnmarshalMap(result.Item, &audition); err != nil {
			return response(500, ErrorResponse{Message: "Failed to read stored audition"})
		}
		if audition.PerformanceID != performanceID {
			return response(400, ErrorResponse{Message: "Audition does not belong to performance '" + performanceID + "'"})
		}
		if audition.Status != pendingStatus {
			return response(409, ErrorResponse{Message: "Audition status must be pending before casting"})
		}

		updatedResult, err := client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName:           aws.String(auditionsTable),
			Key:                 key,
			UpdateExpression:    aws.String("SET #status = :cast"),
			ConditionExpression: aws.String("#status = :pending"),
			ExpressionAttributeNames: map[string]string{
				"#status": "status",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":cast":    &types.AttributeValueMemberS{Value: castStatus},
				":pending": &types.AttributeValueMemberS{Value: pendingStatus},
			},
			ReturnValues: types.ReturnValueAllNew,
		})
		if err != nil {
			var conditionFailed *types.ConditionalCheckFailedException
			if errors.As(err, &conditionFailed) {
				return response(409, ErrorResponse{Message: "Audition status must be pending before casting"})
			}
			return response(500, ErrorResponse{Message: "Failed to cast performer"})
		}
		if len(updatedResult.Attributes) == 0 {
			return response(500, ErrorResponse{Message: "Failed to read updated audition"})
		}

		var updatedAudition Audition
		if err := attributevalue.UnmarshalMap(updatedResult.Attributes, &updatedAudition); err != nil {
			return response(500, ErrorResponse{Message: "Failed to read updated audition"})
		}

		return response(200, CastPerformerResponse{
			Audition: updatedAudition,
			Message:  "Performer cast successfully",
		})
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
	auditionsTable := os.Getenv("AUDITIONS_TABLE_NAME")
	cfg, err := config.LoadDefaultConfig(context.Background())
	var client castDynamoDBAPI
	if err == nil {
		client = newDynamoDBClient(cfg)
	}
	lambda.Start(makeHandler(client, auditionsTable, err))
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
