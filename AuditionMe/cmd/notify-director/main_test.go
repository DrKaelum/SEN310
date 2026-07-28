package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakeNotifyClient struct {
	inputs      []*dynamodb.UpdateItemInput
	errorsByID  map[string]error
	processedID []string
}

func (f *fakeNotifyClient) UpdateItem(_ context.Context, input *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	f.inputs = append(f.inputs, input)
	id := input.Key["Id"].(*types.AttributeValueMemberS).Value
	f.processedID = append(f.processedID, id)
	if err := f.errorsByID[id]; err != nil {
		return nil, err
	}
	return &dynamodb.UpdateItemOutput{}, nil
}

func TestNotifyDirectorProcessesMultipleValidRecords(t *testing.T) {
	client := &fakeNotifyClient{}
	result := callNotify(t, client, events.SQSEvent{Records: []events.SQSMessage{
		validRecord("message-1", "audition-1", "performer-1"),
		validRecord("message-2", "audition-2", "performer-2"),
	}})

	if len(result.BatchItemFailures) != 0 {
		t.Fatalf("batch failures = %v, want none", result.BatchItemFailures)
	}
	if len(client.inputs) != 2 || client.processedID[0] != "audition-1" || client.processedID[1] != "audition-2" {
		t.Fatalf("processed = %v, want both records in order", client.processedID)
	}
	for _, input := range client.inputs {
		if input.UpdateExpression == nil || *input.UpdateExpression != "SET notified = :true" {
			t.Fatalf("update expression = %v", input.UpdateExpression)
		}
		if input.ConditionExpression == nil || *input.ConditionExpression != "attribute_exists(Id)" {
			t.Fatalf("condition = %v", input.ConditionExpression)
		}
		notified, ok := input.ExpressionAttributeValues[":true"].(*types.AttributeValueMemberBOOL)
		if !ok || !notified.Value {
			t.Fatal(":true must be DynamoDB BOOL true")
		}
	}
}

func TestNotifyDirectorContinuesAfterMalformedRecord(t *testing.T) {
	client := &fakeNotifyClient{}
	result := callNotify(t, client, events.SQSEvent{Records: []events.SQSMessage{
		{MessageId: "bad-message", Body: "{"},
		validRecord("good-message", "audition-2", "performer-2"),
	}})

	assertFailureIDs(t, result, []string{"bad-message"})
	if len(client.inputs) != 1 || client.processedID[0] != "audition-2" {
		t.Fatalf("processed = %v, want valid second record", client.processedID)
	}
}

func TestNotifyDirectorMissingAuditionReturnsOnlyThatRecordAsFailed(t *testing.T) {
	client := &fakeNotifyClient{errorsByID: map[string]error{
		"missing-audition": &types.ConditionalCheckFailedException{},
	}}
	result := callNotify(t, client, events.SQSEvent{Records: []events.SQSMessage{
		validRecord("missing-message", "missing-audition", "performer-1"),
		validRecord("good-message", "audition-2", "performer-2"),
	}})

	assertFailureIDs(t, result, []string{"missing-message"})
	if len(client.inputs) != 2 {
		t.Fatalf("UpdateItem calls = %d, want both records processed", len(client.inputs))
	}
}

func TestNotifyDirectorDynamoDBFailureUsesCorrectMessageIdentifier(t *testing.T) {
	client := &fakeNotifyClient{errorsByID: map[string]error{
		"audition-1": errors.New("update failed"),
	}}
	result := callNotify(t, client, events.SQSEvent{Records: []events.SQSMessage{
		validRecord("failed-message-id", "audition-1", "performer-1"),
	}})
	assertFailureIDs(t, result, []string{"failed-message-id"})
}

func TestNotifyDirectorValidatesRequiredMessageFieldsAndStillProcessesAllRecords(t *testing.T) {
	client := &fakeNotifyClient{}
	result := callNotify(t, client, events.SQSEvent{Records: []events.SQSMessage{
		{MessageId: "wrong-event", Body: `{"event_type":"other","auditionId":"a-1","performerId":"p-1"}`},
		{MessageId: "missing-audition", Body: `{"event_type":"audition_created","performerId":"p-1"}`},
		{MessageId: "missing-performer", Body: `{"event_type":"audition_created","auditionId":"a-1"}`},
		validRecord("good-message", "audition-4", "performer-4"),
	}})
	assertFailureIDs(t, result, []string{"wrong-event", "missing-audition", "missing-performer"})
	if len(client.inputs) != 1 || client.processedID[0] != "audition-4" {
		t.Fatalf("processed = %v, want final valid record", client.processedID)
	}
}

func validRecord(messageID string, auditionID string, performerID string) events.SQSMessage {
	return events.SQSMessage{
		MessageId: messageID,
		Body: `{"event_type":"audition_created","auditionId":"` + auditionID +
			`","performerId":"` + performerID + `","timestamp":"2026-07-28T12:00:00Z"}`,
	}
}

func callNotify(t *testing.T, client *fakeNotifyClient, event events.SQSEvent) events.SQSEventResponse {
	t.Helper()
	handler := makeHandler(client, "AuditionsTable", nil)
	result, err := handler(context.Background(), event)
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	return result
}

func assertFailureIDs(t *testing.T, result events.SQSEventResponse, want []string) {
	t.Helper()
	if len(result.BatchItemFailures) != len(want) {
		t.Fatalf("failures = %v, want %v", result.BatchItemFailures, want)
	}
	for index, identifier := range want {
		if result.BatchItemFailures[index].ItemIdentifier != identifier {
			t.Fatalf("failure[%d] = %q, want %q", index, result.BatchItemFailures[index].ItemIdentifier, identifier)
		}
	}
}
