package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakeCastClient struct {
	audition       *Audition
	getErr         error
	updateErr      error
	updated        *Audition
	operations     []string
	getInput       *dynamodb.GetItemInput
	updateInput    *dynamodb.UpdateItemInput
	returnNoFields bool
}

func (f *fakeCastClient) GetItem(_ context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.operations = append(f.operations, "GetItem")
	f.getInput = input
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.audition == nil {
		return &dynamodb.GetItemOutput{}, nil
	}
	item, err := attributevalue.MarshalMap(f.audition)
	if err != nil {
		return nil, err
	}
	return &dynamodb.GetItemOutput{Item: item}, nil
}

func (f *fakeCastClient) UpdateItem(_ context.Context, input *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	f.operations = append(f.operations, "UpdateItem")
	f.updateInput = input
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	if f.returnNoFields {
		return &dynamodb.UpdateItemOutput{}, nil
	}
	updated := f.updated
	if updated == nil {
		copyOfAudition := *f.audition
		copyOfAudition.Status = castStatus
		updated = &copyOfAudition
	}
	attributes, err := attributevalue.MarshalMap(updated)
	if err != nil {
		return nil, err
	}
	return &dynamodb.UpdateItemOutput{Attributes: attributes}, nil
}

func TestCastPerformerReadsThenConditionallyUpdatesAndReturnsAllNew(t *testing.T) {
	client := &fakeCastClient{
		audition: &Audition{
			ID:            "audition-1",
			PerformanceID: "performance-1",
			PerformerID:   "performer-1",
			CharacterName: "Emily",
			Status:        pendingStatus,
		},
		updated: &Audition{
			ID:            "audition-1",
			PerformanceID: "performance-1",
			PerformerID:   "performer-1",
			CharacterName: "Emily",
			Status:        castStatus,
		},
	}

	result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`)
	if result.StatusCode != 200 {
		t.Fatalf("status = %d, want 200; body = %s", result.StatusCode, result.Body)
	}
	if len(client.operations) != 2 || client.operations[0] != "GetItem" || client.operations[1] != "UpdateItem" {
		t.Fatalf("operations = %v, want [GetItem UpdateItem]", client.operations)
	}
	if client.getInput.ConsistentRead == nil || !*client.getInput.ConsistentRead {
		t.Fatal("GetItem must use ConsistentRead=true")
	}
	if client.updateInput.UpdateExpression == nil || *client.updateInput.UpdateExpression != "SET #status = :cast" {
		t.Fatalf("UpdateExpression = %v", client.updateInput.UpdateExpression)
	}
	if client.updateInput.ConditionExpression == nil || *client.updateInput.ConditionExpression != "#status = :pending" {
		t.Fatalf("ConditionExpression = %v", client.updateInput.ConditionExpression)
	}
	if client.updateInput.ReturnValues != types.ReturnValueAllNew {
		t.Fatalf("ReturnValues = %q, want ALL_NEW", client.updateInput.ReturnValues)
	}

	var responseBody CastPerformerResponse
	if err := json.Unmarshal([]byte(result.Body), &responseBody); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	if responseBody.Status != castStatus {
		t.Fatalf("response status = %q, want cast", responseBody.Status)
	}
}

func TestCastPerformerMissingAuditionReturns404WithoutUpdate(t *testing.T) {
	client := &fakeCastClient{}
	result := callCast(t, client, "performance-1", `{"auditionId":"missing"}`)
	if result.StatusCode != 404 {
		t.Fatalf("status = %d, want 404; body = %s", result.StatusCode, result.Body)
	}
	assertNoUpdate(t, client)
}

func TestCastPerformerRejectsPerformanceMismatchWithoutUpdate(t *testing.T) {
	client := pendingAuditionClient()
	result := callCast(t, client, "different-performance", `{"auditionId":"audition-1"}`)
	if result.StatusCode != 400 {
		t.Fatalf("status = %d, want 400; body = %s", result.StatusCode, result.Body)
	}
	assertNoUpdate(t, client)
}

func TestCastPerformerRejectsNonPendingStatusesWithoutUpdate(t *testing.T) {
	for _, status := range []string{"cast", "rejected"} {
		t.Run(status, func(t *testing.T) {
			client := pendingAuditionClient()
			client.audition.Status = status
			result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`)
			if result.StatusCode != 409 {
				t.Fatalf("status = %d, want 409; body = %s", result.StatusCode, result.Body)
			}
			assertNoUpdate(t, client)
		})
	}
}

func TestCastPerformerMapsConditionalFailureTo409(t *testing.T) {
	client := pendingAuditionClient()
	client.updateErr = &types.ConditionalCheckFailedException{Message: stringPointer("status changed")}
	result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`)
	if result.StatusCode != 409 {
		t.Fatalf("status = %d, want 409; body = %s", result.StatusCode, result.Body)
	}
}

func TestCastPerformerReturns500WhenAllNewAttributesAreMissing(t *testing.T) {
	client := pendingAuditionClient()
	client.returnNoFields = true
	result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`)
	if result.StatusCode != 500 {
		t.Fatalf("status = %d, want 500; body = %s", result.StatusCode, result.Body)
	}
}

func TestCastPerformerRejectsInvalidRequestBeforeDynamoDB(t *testing.T) {
	tests := []struct {
		name          string
		performanceID string
		body          string
	}{
		{name: "missing path parameter", body: `{"auditionId":"audition-1"}`},
		{name: "missing body", performanceID: "performance-1"},
		{name: "invalid JSON", performanceID: "performance-1", body: `{`},
		{name: "missing auditionId", performanceID: "performance-1", body: `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := pendingAuditionClient()
			result := callCast(t, client, test.performanceID, test.body)
			if result.StatusCode != 400 {
				t.Fatalf("status = %d, want 400; body = %s", result.StatusCode, result.Body)
			}
			if len(client.operations) != 0 {
				t.Fatalf("operations = %v, want none", client.operations)
			}
		})
	}
}

func TestCastPerformerDynamoDBErrorsReturn500(t *testing.T) {
	t.Run("get failure", func(t *testing.T) {
		client := pendingAuditionClient()
		client.getErr = errors.New("get failed")
		result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`)
		if result.StatusCode != 500 {
			t.Fatalf("status = %d, want 500; body = %s", result.StatusCode, result.Body)
		}
	})
	t.Run("update failure", func(t *testing.T) {
		client := pendingAuditionClient()
		client.updateErr = errors.New("update failed")
		result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`)
		if result.StatusCode != 500 {
			t.Fatalf("status = %d, want 500; body = %s", result.StatusCode, result.Body)
		}
	})
}

func pendingAuditionClient() *fakeCastClient {
	return &fakeCastClient{audition: &Audition{
		ID:            "audition-1",
		PerformanceID: "performance-1",
		PerformerID:   "performer-1",
		CharacterName: "Emily",
		Status:        pendingStatus,
	}}
}

func callCast(t *testing.T, client *fakeCastClient, performanceID string, body string) events.APIGatewayProxyResponse {
	t.Helper()
	handler := makeHandler(client, "AuditionsTable", nil)
	result, err := handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:     "POST",
		PathParameters: map[string]string{"performanceId": performanceID},
		Body:           body,
	})
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	return result
}

func assertNoUpdate(t *testing.T, client *fakeCastClient) {
	t.Helper()
	if client.updateInput != nil {
		t.Fatal("UpdateItem must not be called")
	}
}

func stringPointer(value string) *string {
	return &value
}
