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
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const notifyAction = "notify_director"

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

type AuditionCreatedMessage struct {
	EventType   string `json:"event_type"`
	AuditionID  string `json:"auditionId"`
	PerformerID string `json:"performerId"`
	Timestamp   string `json:"timestamp"`
}

type updateItemAPI interface {
	UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

func makeHandler(client updateItemAPI, auditionsTable string, configErr error) func(context.Context, events.SQSEvent) (events.SQSEventResponse, error) {
	return func(ctx context.Context, event events.SQSEvent) (result events.SQSEventResponse, handlerErr error) {
		successCount := 0
		failureCount := 0
		invocationOutcome := "batch_processed"
		logger.InfoContext(ctx, "Lambda invocation started",
			"action", notifyAction,
			"phase", "start",
			"recordCount", len(event.Records),
		)
		defer func() {
			logger.InfoContext(ctx, "Lambda invocation completed",
				"action", notifyAction,
				"phase", "complete",
				"outcome", invocationOutcome,
				"successCount", successCount,
				"failureCount", failureCount,
			)
		}()

		result.BatchItemFailures = make([]events.SQSBatchItemFailure, 0)
		if auditionsTable == "" || configErr != nil || client == nil {
			invocationOutcome = "configuration_failed"
			errorMessage := "AUDITIONS_TABLE_NAME is not set"
			if configErr != nil {
				errorMessage = configErr.Error()
			} else if client == nil {
				errorMessage = "DynamoDB client is not initialized"
			}
			for _, record := range event.Records {
				result.BatchItemFailures = append(result.BatchItemFailures, events.SQSBatchItemFailure{
					ItemIdentifier: record.MessageId,
				})
				failureCount++
			}
			logger.ErrorContext(ctx, "Lambda invocation outcome",
				"action", notifyAction,
				"phase", "outcome",
				"outcome", invocationOutcome,
				"error", errorMessage,
				"failureCount", failureCount,
			)
			return result, nil
		}

		for _, record := range event.Records {
			message := AuditionCreatedMessage{}
			if err := json.Unmarshal([]byte(record.Body), &message); err != nil {
				failureCount++
				result.BatchItemFailures = append(result.BatchItemFailures, events.SQSBatchItemFailure{
					ItemIdentifier: record.MessageId,
				})
				logger.ErrorContext(ctx, "SQS record failed",
					"action", notifyAction,
					"phase", "outcome",
					"outcome", "invalid_message_json",
					"messageId", record.MessageId,
					"error", err.Error(),
				)
				continue
			}
			message.EventType = strings.TrimSpace(message.EventType)
			message.AuditionID = strings.TrimSpace(message.AuditionID)
			message.PerformerID = strings.TrimSpace(message.PerformerID)

			logger.InfoContext(ctx, "SQS record received",
				"action", notifyAction,
				"phase", "receipt",
				"outcome", "message_received",
				"messageId", record.MessageId,
				"event_type", message.EventType,
				"auditionId", message.AuditionID,
				"performerId", message.PerformerID,
			)

			validationError := ""
			switch {
			case message.EventType != "audition_created":
				validationError = "event_type must be audition_created"
			case message.AuditionID == "":
				validationError = "auditionId is required"
			case message.PerformerID == "":
				validationError = "performerId is required"
			}
			if validationError != "" {
				failureCount++
				result.BatchItemFailures = append(result.BatchItemFailures, events.SQSBatchItemFailure{
					ItemIdentifier: record.MessageId,
				})
				logger.ErrorContext(ctx, "SQS record failed",
					"action", notifyAction,
					"phase", "outcome",
					"outcome", "message_validation_failed",
					"messageId", record.MessageId,
					"auditionId", message.AuditionID,
					"performerId", message.PerformerID,
					"error", validationError,
				)
				continue
			}

			_, err := client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
				TableName: aws.String(auditionsTable),
				Key: map[string]types.AttributeValue{
					"Id": &types.AttributeValueMemberS{Value: message.AuditionID},
				},
				UpdateExpression:    aws.String("SET notified = :true"),
				ConditionExpression: aws.String("attribute_exists(Id)"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":true": &types.AttributeValueMemberBOOL{Value: true},
				},
			})
			if err != nil {
				failureCount++
				result.BatchItemFailures = append(result.BatchItemFailures, events.SQSBatchItemFailure{
					ItemIdentifier: record.MessageId,
				})
				logger.ErrorContext(ctx, "SQS record failed",
					"action", notifyAction,
					"phase", "outcome",
					"outcome", "notification_update_failed",
					"messageId", record.MessageId,
					"auditionId", message.AuditionID,
					"performerId", message.PerformerID,
					"error", err.Error(),
				)
				continue
			}

			successCount++
			logger.InfoContext(ctx, "Audition marked notified",
				"action", notifyAction,
				"phase", "outcome",
				"outcome", "marked_notified",
				"messageId", record.MessageId,
				"auditionId", message.AuditionID,
				"performerId", message.PerformerID,
			)
		}

		if failureCount > 0 {
			invocationOutcome = "batch_partially_failed"
		}
		logger.InfoContext(ctx, "Lambda invocation outcome",
			"action", notifyAction,
			"phase", "outcome",
			"outcome", invocationOutcome,
			"successCount", successCount,
			"failureCount", failureCount,
		)
		return result, nil
	}
}

func main() {
	auditionsTable := os.Getenv("AUDITIONS_TABLE_NAME")
	cfg, err := config.LoadDefaultConfig(context.Background())
	var client updateItemAPI
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
