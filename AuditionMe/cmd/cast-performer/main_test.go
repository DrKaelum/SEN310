package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakeCastClient struct {
	performanceFound bool
	audition         *Audition
	getErrByTable    map[string]error
	updateErr        error
	updated          *Audition
	operations       []string
	getInputs        []*dynamodb.GetItemInput
	updateInput      *dynamodb.UpdateItemInput
	returnNoFields   bool
}

func (f *fakeCastClient) GetItem(_ context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	table := *input.TableName
	f.operations = append(f.operations, "GetItem:"+table)
	f.getInputs = append(f.getInputs, input)
	if err := f.getErrByTable[table]; err != nil {
		return nil, err
	}
	if table == "PerformancesTable" {
		if !f.performanceFound {
			return &dynamodb.GetItemOutput{}, nil
		}
		return &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
			"Id": &types.AttributeValueMemberS{Value: "performance-1"},
		}}, nil
	}
	if f.audition == nil {
		return &dynamodb.GetItemOutput{}, nil
	}
	item, err := attributevalue.MarshalMap(f.audition)
	return &dynamodb.GetItemOutput{Item: item}, err
}

func (f *fakeCastClient) UpdateItem(_ context.Context, input *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	f.operations = append(f.operations, "UpdateItem:"+*input.TableName)
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
	return &dynamodb.UpdateItemOutput{Attributes: attributes}, err
}

func pendingAudition() *Audition {
	return &Audition{
		ID: "audition-1", PerformanceID: "performance-1", PerformerID: "performer-1",
		CharacterName: "Emily", Status: pendingStatus,
	}
}

func TestCastPerformerUsesRequiredOperationOrderAndReturnsAllNew(t *testing.T) {
	client := &fakeCastClient{performanceFound: true, audition: pendingAudition()}
	result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`)

	if result.StatusCode != 200 {
		t.Fatalf("status = %d, want 200; body = %s", result.StatusCode, result.Body)
	}
	want := []string{"GetItem:PerformancesTable", "GetItem:AuditionsTable", "UpdateItem:AuditionsTable"}
	if len(client.operations) != len(want) {
		t.Fatalf("operations = %v, want %v", client.operations, want)
	}
	for index := range want {
		if client.operations[index] != want[index] {
			t.Fatalf("operations = %v, want %v", client.operations, want)
		}
	}
	for _, input := range client.getInputs {
		if input.ConsistentRead == nil || !*input.ConsistentRead {
			t.Fatal("both GetItem calls must use ConsistentRead=true")
		}
	}
	if client.updateInput.ConditionExpression == nil || *client.updateInput.ConditionExpression != "#status = :pending" {
		t.Fatalf("condition = %v, want #status = :pending", client.updateInput.ConditionExpression)
	}
	if client.updateInput.ReturnValues != types.ReturnValueAllNew {
		t.Fatalf("ReturnValues = %s, want ALL_NEW", client.updateInput.ReturnValues)
	}
}

func TestCastPerformerMissingPerformanceReturns404BeforeAuditionRead(t *testing.T) {
	client := &fakeCastClient{}
	result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`)
	if result.StatusCode != 404 {
		t.Fatalf("status = %d, want 404; body = %s", result.StatusCode, result.Body)
	}
	if len(client.operations) != 1 || client.operations[0] != "GetItem:PerformancesTable" {
		t.Fatalf("operations = %v, want only performance GetItem", client.operations)
	}
}

func TestCastPerformerMissingAuditionReturns404WithoutUpdate(t *testing.T) {
	client := &fakeCastClient{performanceFound: true}
	result := callCast(t, client, "performance-1", `{"auditionId":"missing"}`)
	if result.StatusCode != 404 || len(client.operations) != 2 {
		t.Fatalf("status/operations = %d %v, want 404 after two reads", result.StatusCode, client.operations)
	}
}

func TestCastPerformerRejectsPerformanceMismatchWithoutUpdate(t *testing.T) {
	audition := pendingAudition()
	audition.PerformanceID = "another-performance"
	client := &fakeCastClient{performanceFound: true, audition: audition}
	result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`)
	if result.StatusCode != 400 || len(client.operations) != 2 {
		t.Fatalf("status/operations = %d %v, want 400 after two reads", result.StatusCode, client.operations)
	}
}

func TestCastPerformerRejectsNonPendingStatusesWithoutUpdate(t *testing.T) {
	for _, status := range []string{"cast", "rejected"} {
		t.Run(status, func(t *testing.T) {
			audition := pendingAudition()
			audition.Status = status
			client := &fakeCastClient{performanceFound: true, audition: audition}
			result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`)
			if result.StatusCode != 409 || len(client.operations) != 2 {
				t.Fatalf("status/operations = %d %v, want 409 without update", result.StatusCode, client.operations)
			}
		})
	}
}

func TestCastPerformerMapsConditionalFailureTo409(t *testing.T) {
	client := &fakeCastClient{
		performanceFound: true,
		audition:         pendingAudition(),
		updateErr:        &types.ConditionalCheckFailedException{},
	}
	result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`)
	if result.StatusCode != 409 {
		t.Fatalf("status = %d, want 409", result.StatusCode)
	}
}

func TestCastPerformerRejectsInvalidRequestBeforeDynamoDB(t *testing.T) {
	cases := []struct {
		name          string
		performanceID string
		body          string
	}{
		{"missing performanceId", " ", `{"auditionId":"audition-1"}`},
		{"missing body", "performance-1", ""},
		{"invalid JSON", "performance-1", "{"},
		{"missing auditionId", "performance-1", `{}`},
		{"blank auditionId", "performance-1", `{"auditionId":" "}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			client := &fakeCastClient{}
			result := callCast(t, client, testCase.performanceID, testCase.body)
			if result.StatusCode != 400 || len(client.operations) != 0 {
				t.Fatalf("status/operations = %d %v, want 400 and none", result.StatusCode, client.operations)
			}
		})
	}
}

func TestCastPerformerMapsDynamoDBFailuresTo500(t *testing.T) {
	t.Run("performance read", func(t *testing.T) {
		client := &fakeCastClient{getErrByTable: map[string]error{"PerformancesTable": errors.New("read failed")}}
		if result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`); result.StatusCode != 500 {
			t.Fatalf("status = %d, want 500", result.StatusCode)
		}
	})
	t.Run("audition read", func(t *testing.T) {
		client := &fakeCastClient{
			performanceFound: true,
			getErrByTable:    map[string]error{"AuditionsTable": errors.New("read failed")},
		}
		if result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`); result.StatusCode != 500 {
			t.Fatalf("status = %d, want 500", result.StatusCode)
		}
	})
	t.Run("update", func(t *testing.T) {
		client := &fakeCastClient{performanceFound: true, audition: pendingAudition(), updateErr: errors.New("update failed")}
		if result := callCast(t, client, "performance-1", `{"auditionId":"audition-1"}`); result.StatusCode != 500 {
			t.Fatalf("status = %d, want 500", result.StatusCode)
		}
	})
}

func callCast(t *testing.T, client *fakeCastClient, performanceID string, body string) events.APIGatewayProxyResponse {
	t.Helper()
	handler := makeHandler(client, "PerformancesTable", "AuditionsTable", nil)
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
