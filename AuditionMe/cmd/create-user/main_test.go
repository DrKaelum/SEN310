package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type fakePutItemClient struct {
	input *dynamodb.PutItemInput
}

func (f *fakePutItemClient) PutItem(_ context.Context, input *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.input = input
	return &dynamodb.PutItemOutput{}, nil
}

func TestCreateUserStoresRequiredFieldsWithoutPassword(t *testing.T) {
	client := &fakePutItemClient{}
	handler := makeHandler(client, "AuditionMe_Users_davian", nil)
	result, err := handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod: "POST",
		Body:       `{"name":"Avery Stone","email":"avery@example.com","phone":"555-0101","role":"performer","password":"secret"}`,
	})
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	if result.StatusCode != 200 {
		t.Fatalf("status = %d, want 200; body = %s", result.StatusCode, result.Body)
	}
	if client.input == nil {
		t.Fatal("PutItem was not called")
	}
	if got := *client.input.TableName; got != "AuditionMe_Users_davian" {
		t.Fatalf("table = %q, want AuditionMe_Users_davian", got)
	}
	if _, found := client.input.Item["password"]; found {
		t.Fatal("password must not be stored")
	}
	if len(client.input.Item) != 6 {
		t.Fatalf("stored field count = %d, want exactly 6", len(client.input.Item))
	}

	var stored User
	if err := attributevalue.UnmarshalMap(client.input.Item, &stored); err != nil {
		t.Fatalf("could not decode stored user: %v", err)
	}
	if stored.ID == "" || stored.CreatedAt == "" {
		t.Fatalf("generated fields are missing: %+v", stored)
	}
	if stored.Name != "Avery Stone" || stored.Email != "avery@example.com" || stored.Phone != "555-0101" || stored.Role != "performer" {
		t.Fatalf("stored user does not match request: %+v", stored)
	}
}

func TestCreateUserMissingTableNameReturns500WithoutWrite(t *testing.T) {
	client := &fakePutItemClient{}
	handler := makeHandler(client, "", nil)
	result, err := handler(context.Background(), events.APIGatewayProxyRequest{HTTPMethod: "POST", Body: `{}`})
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	if result.StatusCode != 500 {
		t.Fatalf("status = %d, want 500", result.StatusCode)
	}
	if client.input != nil {
		t.Fatal("PutItem must not be called when TABLE_NAME is missing")
	}
}
