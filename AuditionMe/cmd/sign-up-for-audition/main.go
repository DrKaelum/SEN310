package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/google/uuid"
)

const (
	pendingStatus = "pending"
	signUpAction  = "sign_up_for_audition"
)

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

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

type AuditionCreatedMessage struct {
	EventType   string `json:"event_type"`
	AuditionID  string `json:"auditionId"`
	PerformerID string `json:"performerId"`
	Timestamp   string `json:"timestamp"`
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

type sendMessageAPI interface {
	SendMessage(context.Context, *sqs.SendMessageInput, ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
}

func makeHandler(dynamoClient auditionDynamoDBAPI, sqsClient sendMessageAPI, performancesTable string, auditionsTable string, queueURL string, configErr error) func(context.Context, events.APIGatewayProxyRequest) (result events.APIGatewayProxyResponse, handlerErr error) {
	return func(ctx context.Context, event events.APIGatewayProxyRequest) (result events.APIGatewayProxyResponse, handlerErr error) {
		outcome := "unhandled"
		statusCode := 500
		performanceID := ""
		performerID := ""
		auditionID := ""
		logger.InfoContext(ctx, "Lambda invocation started",
			"action", signUpAction,
			"phase", "start",
			"requestId", event.RequestContext.RequestID,
		)
		defer func() {
			logger.InfoContext(ctx, "Lambda invocation completed",
				"action", signUpAction,
				"phase", "complete",
				"outcome", outcome,
				"statusCode", statusCode,
				"performanceId", performanceID,
				"performerId", performerID,
				"auditionId", auditionID,
			)
		}()
		finish := func(code int, resultOutcome string, body any, logValues ...any) (events.APIGatewayProxyResponse, error) {
			outcome = resultOutcome
			statusCode = code
			values := []any{
				"action", signUpAction,
				"phase", "outcome",
				"outcome", resultOutcome,
				"statusCode", code,
				"performanceId", performanceID,
				"performerId", performerID,
				"auditionId", auditionID,
			}
			values = append(values, logValues...)
			if code >= 500 {
				logger.ErrorContext(ctx, "Lambda invocation outcome", values...)
			} else {
				logger.InfoContext(ctx, "Lambda invocation outcome", values...)
			}
			return response(code, body)
		}

		if event.HTTPMethod == "OPTIONS" {
			return finish(200, "cors_preflight", map[string]string{"message": "CORS preflight OK"})
		}

		body, bodyError := requestBody(event)
		if bodyError != "" {
			return finish(400, "validation_failed", ErrorResponse{Message: bodyError}, "error", bodyError)
		}
		var request SignUpRequest
		if err := json.Unmarshal([]byte(body), &request); err != nil {
			return finish(400, "validation_failed", ErrorResponse{Message: "Invalid request body: expected JSON"}, "error", err.Error())
		}
		performanceID = strings.TrimSpace(request.PerformanceID)
		performerID = strings.TrimSpace(request.PerformerID)
		request.CharacterName = strings.TrimSpace(request.CharacterName)
		if performanceID == "" {
			return finish(400, "validation_failed", ErrorResponse{Message: "Missing required field: performanceId"}, "error", "performanceId is required")
		}
		if performerID == "" {
			return finish(400, "validation_failed", ErrorResponse{Message: "Missing required field: performerId"}, "error", "performerId is required")
		}
		if request.CharacterName == "" {
			return finish(400, "validation_failed", ErrorResponse{Message: "Missing required field: characterName"}, "error", "characterName is required")
		}
		if performancesTable == "" {
			return finish(500, "configuration_failed", ErrorResponse{Message: "Server configuration error: PERFORMANCES_TABLE_NAME is not set"}, "error", "PERFORMANCES_TABLE_NAME is not set")
		}
		if auditionsTable == "" {
			return finish(500, "configuration_failed", ErrorResponse{Message: "Server configuration error: AUDITIONS_TABLE_NAME is not set"}, "error", "AUDITIONS_TABLE_NAME is not set")
		}
		if queueURL == "" {
			return finish(500, "configuration_failed", ErrorResponse{Message: "Server configuration error: QUEUE_URL is not set"}, "error", "QUEUE_URL is not set")
		}
		if configErr != nil || dynamoClient == nil || sqsClient == nil {
			errorMessage := "unable to initialize AWS"
			if configErr != nil {
				errorMessage = configErr.Error()
			}
			return finish(500, "configuration_failed", ErrorResponse{Message: "Server configuration error: unable to initialize AWS"}, "error", errorMessage)
		}

		performance, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(performancesTable),
			Key: map[string]types.AttributeValue{
				"Id": &types.AttributeValueMemberS{Value: performanceID},
			},
			ConsistentRead: aws.Bool(true),
		})
		if err != nil {
			return finish(500, "performance_read_failed", ErrorResponse{Message: "Failed to verify performance"}, "error", err.Error())
		}
		if len(performance.Item) == 0 {
			message := "Performance '" + performanceID + "' not found"
			return finish(404, "performance_not_found", ErrorResponse{Message: message}, "error", message)
		}

		audition := Audition{
			ID:            uuid.NewString(),
			PerformanceID: performanceID,
			PerformerID:   performerID,
			CharacterName: request.CharacterName,
			Status:        pendingStatus,
		}
		auditionID = audition.ID
		item, err := attributevalue.MarshalMap(audition)
		if err != nil {
			return finish(500, "audition_encode_failed", ErrorResponse{Message: "Failed to prepare audition for storage"}, "error", err.Error())
		}
		if _, err := dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(auditionsTable),
			Item:      item,
		}); err != nil {
			return finish(500, "audition_write_failed", ErrorResponse{Message: "Failed to store audition"}, "error", err.Error())
		}

		message := AuditionCreatedMessage{
			EventType:   "audition_created",
			AuditionID:  audition.ID,
			PerformerID: audition.PerformerID,
			Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
		}
		messageBody, err := json.Marshal(message)
		if err != nil {
			return finish(500, "notification_encode_failed", ErrorResponse{Message: "Audition was stored, but the notification message could not be prepared"}, "error", err.Error(), "auditionStored", true)
		}
		if _, err := sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl:    aws.String(queueURL),
			MessageBody: aws.String(string(messageBody)),
		}); err != nil {
			return finish(500, "audition_stored_notification_failed", ErrorResponse{
				Message: "Audition was stored, but the notification could not be queued",
			}, "error", err.Error(), "auditionStored", true)
		}

		return finish(200, "audition_created_and_queued", SignUpResponse{
			Audition: audition,
			Message:  "Audition sign-up created successfully",
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
			"Access-Control-Allow-Methods": "OPTIONS,POST,GET,DELETE",
		},
		Body: string(bodyJSON),
	}, nil
}

func main() {
	performancesTable := os.Getenv("PERFORMANCES_TABLE_NAME")
	auditionsTable := os.Getenv("AUDITIONS_TABLE_NAME")
	queueURL := os.Getenv("QUEUE_URL")
	cfg, err := config.LoadDefaultConfig(context.Background())
	var dynamoClient auditionDynamoDBAPI
	var sqsClient sendMessageAPI
	if err == nil {
		dynamoClient = newDynamoDBClient(cfg)
		sqsClient = newSQSClient(cfg)
	}
	lambda.Start(makeHandler(dynamoClient, sqsClient, performancesTable, auditionsTable, queueURL, err))
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

func newSQSClient(cfg aws.Config) *sqs.Client {
	endpoint := strings.TrimSpace(os.Getenv("SQS_ENDPOINT"))
	if endpoint == "" {
		return sqs.NewFromConfig(cfg)
	}
	return sqs.NewFromConfig(cfg, func(options *sqs.Options) {
		options.BaseEndpoint = aws.String(endpoint)
	})
}
