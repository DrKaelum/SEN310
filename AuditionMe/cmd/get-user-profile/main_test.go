package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type fakeGetUserClient struct {
	user  *User
	err   error
	input *dynamodb.GetItemInput
}

func (f *fakeGetUserClient) GetItem(_ context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	if f.user == nil {
		return &dynamodb.GetItemOutput{}, nil
	}
	item, err := attributevalue.MarshalMap(f.user)
	return &dynamodb.GetItemOutput{Item: item}, err
}

func TestGetUserProfileReturnsCompleteProfileWithoutPassword(t *testing.T) {
	client := &fakeGetUserClient{user: &User{
		ID: "user-1", Name: "Avery Stone", Email: "avery@example.com",
		Phone: "555-0101", Role: "performer", CreatedAt: "2026-07-28T12:00:00Z",
	}}
	result := callGetUser(t, client, "user-1")

	if result.StatusCode != 200 {
		t.Fatalf("status = %d, want 200; body = %s", result.StatusCode, result.Body)
	}
	if client.input == nil || client.input.ConsistentRead == nil || !*client.input.ConsistentRead {
		t.Fatal("GetItem must use ConsistentRead=true")
	}
	if *client.input.TableName != "UsersTable" {
		t.Fatalf("table = %q, want UsersTable", *client.input.TableName)
	}
	if strings.Contains(result.Body, "password") {
		t.Fatalf("profile response must not contain a password: %s", result.Body)
	}
	if !strings.Contains(result.Body, `"Id":"user-1"`) || !strings.Contains(result.Body, `"created_at":"2026-07-28T12:00:00Z"`) {
		t.Fatalf("response is missing profile fields: %s", result.Body)
	}
}

func TestGetUserProfileReturns404WhenMissing(t *testing.T) {
	result := callGetUser(t, &fakeGetUserClient{}, "missing-user")
	if result.StatusCode != 404 || !strings.Contains(result.Body, "missing-user") {
		t.Fatalf("status/body = %d %s, want user-specific 404", result.StatusCode, result.Body)
	}
}

func TestGetUserProfileRejectsBlankUserID(t *testing.T) {
	client := &fakeGetUserClient{}
	result := callGetUser(t, client, " ")
	if result.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", result.StatusCode)
	}
	if client.input != nil {
		t.Fatal("GetItem must not be called for a blank userId")
	}
}

func TestGetUserProfileMapsDynamoDBFailureTo500(t *testing.T) {
	result := callGetUser(t, &fakeGetUserClient{err: errors.New("read failed")}, "user-1")
	if result.StatusCode != 500 {
		t.Fatalf("status = %d, want 500", result.StatusCode)
	}
}

func callGetUser(t *testing.T, client *fakeGetUserClient, userID string) events.APIGatewayProxyResponse {
	t.Helper()
	handler := makeHandler(client, "UsersTable", nil)
	result, err := handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:     "GET",
		PathParameters: map[string]string{"userId": userID},
	})
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	return result
}
