package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type fakeAuditionClient struct {
	performanceFound bool
	getErr           error
	putErr           error
	operations       *[]string
	getInput         *dynamodb.GetItemInput
	putInput         *dynamodb.PutItemInput
}

func (f *fakeAuditionClient) GetItem(_ context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	*f.operations = append(*f.operations, "GetItem")
	f.getInput = input
	if f.getErr != nil {
		return nil, f.getErr
	}
	if !f.performanceFound {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
		"Id": &types.AttributeValueMemberS{Value: "performance-1"},
	}}, nil
}

func (f *fakeAuditionClient) PutItem(_ context.Context, input *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	*f.operations = append(*f.operations, "PutItem")
	f.putInput = input
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &dynamodb.PutItemOutput{}, nil
}

type fakeSQSClient struct {
	err        error
	operations *[]string
	input      *sqs.SendMessageInput
}

func (f *fakeSQSClient) SendMessage(_ context.Context, input *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	*f.operations = append(*f.operations, "SendMessage")
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return &sqs.SendMessageOutput{}, nil
}

func TestValidAuditionUsesRequiredOrderAndPublishesExactMessageShape(t *testing.T) {
	operations := make([]string, 0)
	dynamoClient := &fakeAuditionClient{performanceFound: true, operations: &operations}
	sqsClient := &fakeSQSClient{operations: &operations}
	result := callSignUp(t, dynamoClient, sqsClient, validSignUpBody)

	if result.StatusCode != 200 {
		t.Fatalf("status = %d, want 200; body = %s", result.StatusCode, result.Body)
	}
	wantOperations := []string{"GetItem", "PutItem", "SendMessage"}
	if strings.Join(operations, ",") != strings.Join(wantOperations, ",") {
		t.Fatalf("operations = %v, want %v", operations, wantOperations)
	}
	if dynamoClient.getInput.ConsistentRead == nil || !*dynamoClient.getInput.ConsistentRead {
		t.Fatal("performance GetItem must use ConsistentRead=true")
	}
	var stored Audition
	if err := attributevalue.UnmarshalMap(dynamoClient.putInput.Item, &stored); err != nil {
		t.Fatalf("decode stored audition: %v", err)
	}
	if stored.Status != pendingStatus || stored.ID == "" {
		t.Fatalf("stored audition = %+v, want generated pending audition", stored)
	}

	var message map[string]string
	if err := json.Unmarshal([]byte(*sqsClient.input.MessageBody), &message); err != nil {
		t.Fatalf("decode SQS body: %v", err)
	}
	if len(message) != 4 {
		t.Fatalf("message fields = %v, want exactly four", message)
	}
	if message["event_type"] != "audition_created" || message["auditionId"] != stored.ID || message["performerId"] != "performer-1" {
		t.Fatalf("message = %v, want audition-created data", message)
	}
	if _, err := time.Parse(time.RFC3339Nano, message["timestamp"]); err != nil {
		t.Fatalf("timestamp %q is not RFC3339: %v", message["timestamp"], err)
	}
	if *sqsClient.input.QueueUrl != "http://queue.local/notification" {
		t.Fatalf("queue URL = %q", *sqsClient.input.QueueUrl)
	}
}

func TestMissingPerformanceReturns404AndNeverWritesOrPublishes(t *testing.T) {
	operations := make([]string, 0)
	dynamoClient := &fakeAuditionClient{operations: &operations}
	sqsClient := &fakeSQSClient{operations: &operations}
	result := callSignUp(t, dynamoClient, sqsClient, validSignUpBody)
	if result.StatusCode != 404 {
		t.Fatalf("status = %d, want 404; body = %s", result.StatusCode, result.Body)
	}
	if strings.Join(operations, ",") != "GetItem" {
		t.Fatalf("operations = %v, want only GetItem", operations)
	}
}

func TestPutFailureNeverPublishes(t *testing.T) {
	operations := make([]string, 0)
	dynamoClient := &fakeAuditionClient{
		performanceFound: true,
		putErr:           errors.New("write failed"),
		operations:       &operations,
	}
	sqsClient := &fakeSQSClient{operations: &operations}
	result := callSignUp(t, dynamoClient, sqsClient, validSignUpBody)
	if result.StatusCode != 500 {
		t.Fatalf("status = %d, want 500", result.StatusCode)
	}
	if strings.Join(operations, ",") != "GetItem,PutItem" {
		t.Fatalf("operations = %v, want no SendMessage", operations)
	}
}

func TestSendFailureReturns500AndClearlySaysAuditionWasStored(t *testing.T) {
	operations := make([]string, 0)
	dynamoClient := &fakeAuditionClient{performanceFound: true, operations: &operations}
	sqsClient := &fakeSQSClient{err: errors.New("queue failed"), operations: &operations}
	result := callSignUp(t, dynamoClient, sqsClient, validSignUpBody)
	if result.StatusCode != 500 {
		t.Fatalf("status = %d, want 500", result.StatusCode)
	}
	if strings.Join(operations, ",") != "GetItem,PutItem,SendMessage" {
		t.Fatalf("operations = %v, want write before failed publish", operations)
	}
	if !strings.Contains(result.Body, "Audition was stored") {
		t.Fatalf("partial-success response must acknowledge storage: %s", result.Body)
	}
	if dynamoClient.putInput == nil {
		t.Fatal("PutItem must have succeeded before SendMessage failure")
	}
}

func TestSignUpValidationPathsDoNotCallAWS(t *testing.T) {
	cases := []string{
		"",
		"{",
		`{}`,
		`{"performanceId":"performance-1"}`,
		`{"performanceId":"performance-1","performerId":"performer-1"}`,
		`{"performanceId":" ","performerId":"performer-1","characterName":"Emily"}`,
		`{"performanceId":"performance-1","performerId":" ","characterName":"Emily"}`,
		`{"performanceId":"performance-1","performerId":"performer-1","characterName":" "}`,
	}
	for _, body := range cases {
		operations := make([]string, 0)
		result := callSignUp(t,
			&fakeAuditionClient{operations: &operations},
			&fakeSQSClient{operations: &operations},
			body,
		)
		if result.StatusCode != 400 {
			t.Fatalf("body %q status = %d, want 400", body, result.StatusCode)
		}
		if len(operations) != 0 {
			t.Fatalf("body %q operations = %v, want none", body, operations)
		}
	}
}

const validSignUpBody = `{"performanceId":"performance-1","performerId":"performer-1","characterName":"Emily"}`

func callSignUp(t *testing.T, dynamoClient *fakeAuditionClient, sqsClient *fakeSQSClient, body string) events.APIGatewayProxyResponse {
	t.Helper()
	handler := makeHandler(
		dynamoClient,
		sqsClient,
		"PerformancesTable",
		"AuditionsTable",
		"http://queue.local/notification",
		nil,
	)
	result, err := handler(context.Background(), events.APIGatewayProxyRequest{HTTPMethod: "POST", Body: body})
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	return result
}
