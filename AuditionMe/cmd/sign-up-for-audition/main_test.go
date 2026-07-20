package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakeAuditionClient struct {
	performanceFound bool
	operations       []string
	getInput         *dynamodb.GetItemInput
	putInput         *dynamodb.PutItemInput
}

func (f *fakeAuditionClient) GetItem(_ context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.operations = append(f.operations, "GetItem")
	f.getInput = input
	if !f.performanceFound {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{"Id": &types.AttributeValueMemberS{Value: "performance-1"}}}, nil
}

func (f *fakeAuditionClient) PutItem(_ context.Context, input *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.operations = append(f.operations, "PutItem")
	f.putInput = input
	return &dynamodb.PutItemOutput{}, nil
}

func TestMissingPerformanceReturns404AndNeverWrites(t *testing.T) {
	client := &fakeAuditionClient{}
	result := callSignUp(t, client)
	if result.StatusCode != 404 {
		t.Fatalf("status = %d, want 404; body = %s", result.StatusCode, result.Body)
	}
	if len(client.operations) != 1 || client.operations[0] != "GetItem" {
		t.Fatalf("operations = %v, want only GetItem", client.operations)
	}
	if client.putInput != nil {
		t.Fatal("PutItem must not be called for a missing performance")
	}
	if client.getInput.ConsistentRead == nil || !*client.getInput.ConsistentRead {
		t.Fatal("GetItem must use ConsistentRead=true")
	}
}

func TestValidAuditionReadsBeforeWriteAndStoresPending(t *testing.T) {
	client := &fakeAuditionClient{performanceFound: true}
	result := callSignUp(t, client)
	if result.StatusCode != 200 {
		t.Fatalf("status = %d, want 200; body = %s", result.StatusCode, result.Body)
	}
	if len(client.operations) != 2 || client.operations[0] != "GetItem" || client.operations[1] != "PutItem" {
		t.Fatalf("operations = %v, want [GetItem PutItem]", client.operations)
	}
	if *client.getInput.TableName != "AuditionMe_Performances_davian" {
		t.Fatalf("GetItem table = %q", *client.getInput.TableName)
	}
	if *client.putInput.TableName != "AuditionMe_Auditions_davian" {
		t.Fatalf("PutItem table = %q", *client.putInput.TableName)
	}
	if len(client.putInput.Item) != 5 {
		t.Fatalf("stored field count = %d, want exactly 5", len(client.putInput.Item))
	}
	var stored Audition
	if err := attributevalue.UnmarshalMap(client.putInput.Item, &stored); err != nil {
		t.Fatalf("could not decode stored audition: %v", err)
	}
	if stored.ID == "" {
		t.Fatal("audition Id was not generated")
	}
	if stored.PerformanceID != "performance-1" || stored.PerformerID != "performer-1" || stored.CharacterName != "Emily" {
		t.Fatalf("stored audition does not match request: %+v", stored)
	}
	if stored.Status != "pending" {
		t.Fatalf("status = %q, want exactly pending", stored.Status)
	}
}

func callSignUp(t *testing.T, client *fakeAuditionClient) events.APIGatewayProxyResponse {
	t.Helper()
	handler := makeHandler(client, "AuditionMe_Performances_davian", "AuditionMe_Auditions_davian", nil)
	result, err := handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod: "POST",
		Body:       `{"performanceId":"performance-1","performerId":"performer-1","characterName":"Emily"}`,
	})
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	return result
}
