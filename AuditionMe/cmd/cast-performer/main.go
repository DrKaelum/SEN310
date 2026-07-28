package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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

const (
	pendingStatus = "pending"
	castStatus    = "cast"
	castAction    = "cast_performer"
)

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

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

func makeHandler(client castDynamoDBAPI, performancesTable string, auditionsTable string, configErr error) func(context.Context, events.APIGatewayProxyRequest) (result events.APIGatewayProxyResponse, handlerErr error) {
	return func(ctx context.Context, event events.APIGatewayProxyRequest) (result events.APIGatewayProxyResponse, handlerErr error) {
		outcome := "unhandled"
		statusCode := 500
		performanceID := strings.TrimSpace(event.PathParameters["performanceId"])
		auditionID := ""
		logger.InfoContext(ctx, "Lambda invocation started",
			"action", castAction,
			"phase", "start",
			"requestId", event.RequestContext.RequestID,
			"performanceId", performanceID,
		)
		defer func() {
			logger.InfoContext(ctx, "Lambda invocation completed",
				"action", castAction,
				"phase", "complete",
				"outcome", outcome,
				"statusCode", statusCode,
				"performanceId", performanceID,
				"auditionId", auditionID,
			)
		}()
		finish := func(code int, resultOutcome string, body any, logValues ...any) (events.APIGatewayProxyResponse, error) {
			outcome = resultOutcome
			statusCode = code
			values := []any{"action", castAction, "phase", "outcome", "outcome", resultOutcome, "statusCode", code, "performanceId", performanceID, "auditionId", auditionID}
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
		if performanceID == "" {
			return finish(400, "validation_failed", ErrorResponse{Message: "Missing required path parameter: performanceId"}, "error", "performanceId is required")
		}

		body, bodyError := requestBody(event)
		if bodyError != "" {
			return finish(400, "validation_failed", ErrorResponse{Message: bodyError}, "error", bodyError)
		}
		var request CastPerformerRequest
		if err := json.Unmarshal([]byte(body), &request); err != nil {
			return finish(400, "validation_failed", ErrorResponse{Message: "Invalid request body: expected JSON"}, "error", err.Error())
		}
		auditionID = strings.TrimSpace(request.AuditionID)
		if auditionID == "" {
			return finish(400, "validation_failed", ErrorResponse{Message: "Missing required field: auditionId"}, "error", "auditionId is required")
		}

		if performancesTable == "" {
			return finish(500, "configuration_failed", ErrorResponse{Message: "Server configuration error: PERFORMANCES_TABLE_NAME is not set"}, "error", "PERFORMANCES_TABLE_NAME is not set")
		}
		if auditionsTable == "" {
			return finish(500, "configuration_failed", ErrorResponse{Message: "Server configuration error: AUDITIONS_TABLE_NAME is not set"}, "error", "AUDITIONS_TABLE_NAME is not set")
		}
		if configErr != nil || client == nil {
			errorMessage := "unable to initialize AWS"
			if configErr != nil {
				errorMessage = configErr.Error()
			}
			return finish(500, "configuration_failed", ErrorResponse{Message: "Server configuration error: unable to initialize AWS"}, "error", errorMessage)
		}

		performanceResult, err := client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(performancesTable),
			Key: map[string]types.AttributeValue{
				"Id": &types.AttributeValueMemberS{Value: performanceID},
			},
			ConsistentRead: aws.Bool(true),
		})
		if err != nil {
			return finish(500, "performance_read_failed", ErrorResponse{Message: "Failed to verify performance"}, "error", err.Error())
		}
		if len(performanceResult.Item) == 0 {
			message := "Performance '" + performanceID + "' not found"
			return finish(404, "performance_not_found", ErrorResponse{Message: message}, "error", message)
		}

		auditionKey := map[string]types.AttributeValue{
			"Id": &types.AttributeValueMemberS{Value: auditionID},
		}
		auditionResult, err := client.GetItem(ctx, &dynamodb.GetItemInput{
			TableName:      aws.String(auditionsTable),
			Key:            auditionKey,
			ConsistentRead: aws.Bool(true),
		})
		if err != nil {
			return finish(500, "audition_read_failed", ErrorResponse{Message: "Failed to read audition"}, "error", err.Error())
		}
		if len(auditionResult.Item) == 0 {
			message := "Audition '" + auditionID + "' not found"
			return finish(404, "audition_not_found", ErrorResponse{Message: message}, "error", message)
		}

		var audition Audition
		if err := attributevalue.UnmarshalMap(auditionResult.Item, &audition); err != nil {
			return finish(500, "audition_decode_failed", ErrorResponse{Message: "Failed to read stored audition"}, "error", err.Error())
		}
		if audition.PerformanceID != performanceID {
			message := "Audition does not belong to performance '" + performanceID + "'"
			return finish(400, "performance_mismatch", ErrorResponse{Message: message}, "error", message)
		}
		if audition.Status != pendingStatus {
			message := "Audition status must be pending before casting"
			return finish(409, "invalid_status_transition", ErrorResponse{Message: message}, "error", message, "currentStatus", audition.Status)
		}

		updatedResult, err := client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName:           aws.String(auditionsTable),
			Key:                 auditionKey,
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
				return finish(409, "invalid_status_transition", ErrorResponse{Message: "Audition status must be pending before casting"}, "error", err.Error())
			}
			return finish(500, "update_failed", ErrorResponse{Message: "Failed to cast performer"}, "error", err.Error())
		}
		if len(updatedResult.Attributes) == 0 {
			return finish(500, "updated_attributes_missing", ErrorResponse{Message: "Failed to read updated audition"}, "error", "UpdateItem returned no attributes")
		}

		var updatedAudition Audition
		if err := attributevalue.UnmarshalMap(updatedResult.Attributes, &updatedAudition); err != nil {
			return finish(500, "updated_audition_decode_failed", ErrorResponse{Message: "Failed to read updated audition"}, "error", err.Error())
		}

		return finish(200, "performer_cast", CastPerformerResponse{
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
			"Access-Control-Allow-Methods": "OPTIONS,POST,GET,DELETE",
		},
		Body: string(bodyJSON),
	}, nil
}

func main() {
	performancesTable := os.Getenv("PERFORMANCES_TABLE_NAME")
	auditionsTable := os.Getenv("AUDITIONS_TABLE_NAME")
	cfg, err := config.LoadDefaultConfig(context.Background())
	var client castDynamoDBAPI
	if err == nil {
		client = newDynamoDBClient(cfg)
	}
	lambda.Start(makeHandler(client, performancesTable, auditionsTable, err))
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
