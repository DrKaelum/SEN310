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

func TestPostPerformanceAcceptsIsLiveFalse(t *testing.T) {
	client := &fakePutItemClient{}
	handler := makeHandler(client, "AuditionMe_Performances_davian", nil)
	result, err := handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod: "POST",
		Body:       `{"title":"Our Town","director":"Dana Lee","castingDirector":"Morgan Ray","venue":"Main Stage","performanceDates":["2026-09-01"],"characters":["Emily"],"isLive":false}`,
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
	var stored Performance
	if err := attributevalue.UnmarshalMap(client.input.Item, &stored); err != nil {
		t.Fatalf("could not decode stored performance: %v", err)
	}
	if stored.IsLive {
		t.Fatal("isLive=false was not preserved")
	}
	if stored.ID == "" || len(client.input.Item) != 8 {
		t.Fatalf("stored performance is incomplete: %+v", stored)
	}
}

func TestPostPerformanceRejectsOmittedIsLive(t *testing.T) {
	assertRejected(t, `{"title":"Our Town","director":"Dana Lee","castingDirector":"Morgan Ray","venue":"Main Stage","performanceDates":["2026-09-01"],"characters":["Emily"]}`)
}

func TestPostPerformanceRejectsEmptyLists(t *testing.T) {
	assertRejected(t, `{"title":"Our Town","director":"Dana Lee","castingDirector":"Morgan Ray","venue":"Main Stage","performanceDates":[],"characters":["Emily"],"isLive":true}`)
	assertRejected(t, `{"title":"Our Town","director":"Dana Lee","castingDirector":"Morgan Ray","venue":"Main Stage","performanceDates":["2026-09-01"],"characters":[],"isLive":true}`)
}

func assertRejected(t *testing.T, body string) {
	t.Helper()
	client := &fakePutItemClient{}
	handler := makeHandler(client, "AuditionMe_Performances_davian", nil)
	result, err := handler(context.Background(), events.APIGatewayProxyRequest{HTTPMethod: "POST", Body: body})
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	if result.StatusCode != 400 {
		t.Fatalf("status = %d, want 400; body = %s", result.StatusCode, result.Body)
	}
	if client.input != nil {
		t.Fatal("PutItem must not be called for invalid input")
	}
}
