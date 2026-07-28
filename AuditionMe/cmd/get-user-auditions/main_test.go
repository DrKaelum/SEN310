package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakeHistoryClient struct {
	scanOutputs      []*dynamodb.ScanOutput
	scanErr          error
	performanceItems map[string]map[string]types.AttributeValue
	getErr           error
	scanInputs       []*dynamodb.ScanInput
	getInputs        []*dynamodb.GetItemInput
}

func (f *fakeHistoryClient) Scan(_ context.Context, input *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	copyOfInput := *input
	f.scanInputs = append(f.scanInputs, &copyOfInput)
	if f.scanErr != nil {
		return nil, f.scanErr
	}
	output := f.scanOutputs[0]
	f.scanOutputs = f.scanOutputs[1:]
	return output, nil
}

func (f *fakeHistoryClient) GetItem(_ context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.getInputs = append(f.getInputs, input)
	if f.getErr != nil {
		return nil, f.getErr
	}
	id := input.Key["Id"].(*types.AttributeValueMemberS).Value
	return &dynamodb.GetItemOutput{Item: f.performanceItems[id]}, nil
}

func TestGetUserAuditionsPaginatesAndEnrichesEveryAudition(t *testing.T) {
	first := mustMarshal(t, Audition{ID: "a-1", PerformanceID: "p-1", PerformerID: "u-1", Status: "pending"})
	second := mustMarshal(t, Audition{ID: "a-2", PerformanceID: "p-2", PerformerID: "u-1", Status: "cast", Notified: true})
	lastKey := map[string]types.AttributeValue{"Id": &types.AttributeValueMemberS{Value: "a-1"}}
	client := &fakeHistoryClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{Items: []map[string]types.AttributeValue{first}, LastEvaluatedKey: lastKey},
			{Items: []map[string]types.AttributeValue{second}},
		},
		performanceItems: map[string]map[string]types.AttributeValue{
			"p-1": {"title": &types.AttributeValueMemberS{Value: "Hamlet"}},
			"p-2": {"title": &types.AttributeValueMemberS{Value: "Our Town"}},
		},
	}
	result := callHistory(t, client, "u-1")

	if result.StatusCode != 200 {
		t.Fatalf("status = %d, want 200; body = %s", result.StatusCode, result.Body)
	}
	if len(client.scanInputs) != 2 {
		t.Fatalf("Scan calls = %d, want 2", len(client.scanInputs))
	}
	if len(client.getInputs) != 2 {
		t.Fatalf("GetItem calls = %d, want one per audition", len(client.getInputs))
	}
	if !strings.Contains(result.Body, `"performanceTitle":"Hamlet"`) || !strings.Contains(result.Body, `"performanceTitle":"Our Town"`) {
		t.Fatalf("titles were not enriched: %s", result.Body)
	}
	if client.scanInputs[0].FilterExpression == nil || *client.scanInputs[0].FilterExpression != "#performerId = :userId" {
		t.Fatalf("unexpected performer filter: %v", client.scanInputs[0].FilterExpression)
	}
}

func TestGetUserAuditionsReturnsEmptyArray(t *testing.T) {
	client := &fakeHistoryClient{scanOutputs: []*dynamodb.ScanOutput{{}}}
	result := callHistory(t, client, "u-1")
	if result.StatusCode != 200 || result.Body != `{"auditions":[]}` {
		t.Fatalf("status/body = %d %s, want empty auditions array", result.StatusCode, result.Body)
	}
	if len(client.getInputs) != 0 {
		t.Fatal("GetItem must not be called when there are no auditions")
	}
}

func TestGetUserAuditionsRetainsAuditionWhenPerformanceIsMissing(t *testing.T) {
	audition := mustMarshal(t, Audition{ID: "a-1", PerformanceID: "deleted", PerformerID: "u-1", Status: "pending"})
	client := &fakeHistoryClient{
		scanOutputs:      []*dynamodb.ScanOutput{{Items: []map[string]types.AttributeValue{audition}}},
		performanceItems: map[string]map[string]types.AttributeValue{},
	}
	result := callHistory(t, client, "u-1")
	if result.StatusCode != 200 || !strings.Contains(result.Body, `"performanceTitle":"Performance unavailable"`) {
		t.Fatalf("status/body = %d %s, want retained audition with placeholder", result.StatusCode, result.Body)
	}
}

func TestGetUserAuditionsRejectsBlankUserID(t *testing.T) {
	client := &fakeHistoryClient{}
	result := callHistory(t, client, " ")
	if result.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", result.StatusCode)
	}
	if len(client.scanInputs) != 0 {
		t.Fatal("Scan must not be called for blank userId")
	}
}

func TestGetUserAuditionsMapsDynamoDBFailuresTo500(t *testing.T) {
	scanResult := callHistory(t, &fakeHistoryClient{scanErr: errors.New("scan failed")}, "u-1")
	if scanResult.StatusCode != 500 {
		t.Fatalf("scan failure status = %d, want 500", scanResult.StatusCode)
	}

	audition := mustMarshal(t, Audition{ID: "a-1", PerformanceID: "p-1", PerformerID: "u-1"})
	getResult := callHistory(t, &fakeHistoryClient{
		scanOutputs: []*dynamodb.ScanOutput{{Items: []map[string]types.AttributeValue{audition}}},
		getErr:      errors.New("get failed"),
	}, "u-1")
	if getResult.StatusCode != 500 {
		t.Fatalf("GetItem failure status = %d, want 500", getResult.StatusCode)
	}
}

func mustMarshal(t *testing.T, value any) map[string]types.AttributeValue {
	t.Helper()
	item, err := attributevalue.MarshalMap(value)
	if err != nil {
		t.Fatalf("marshal test item: %v", err)
	}
	return item
}

func callHistory(t *testing.T, client *fakeHistoryClient, userID string) events.APIGatewayProxyResponse {
	t.Helper()
	handler := makeHandler(client, "AuditionsTable", "PerformancesTable", nil)
	result, err := handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:     "GET",
		PathParameters: map[string]string{"userId": userID},
	})
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	return result
}
