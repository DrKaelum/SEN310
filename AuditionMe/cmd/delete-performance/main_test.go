package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakeDeleteClient struct {
	output *dynamodb.DeleteItemOutput
	err    error
	input  *dynamodb.DeleteItemInput
}

func (f *fakeDeleteClient) DeleteItem(_ context.Context, input *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	f.input = input
	if f.output == nil {
		f.output = &dynamodb.DeleteItemOutput{}
	}
	return f.output, f.err
}

func TestDeletePerformanceUsesAtomicConditionalDelete(t *testing.T) {
	client := &fakeDeleteClient{output: &dynamodb.DeleteItemOutput{
		Attributes: map[string]types.AttributeValue{"Id": &types.AttributeValueMemberS{Value: "performance-1"}},
	}}
	result := callDelete(t, client, "performance-1")

	if result.StatusCode != 200 || !strings.Contains(result.Body, `"performanceId":"performance-1"`) {
		t.Fatalf("status/body = %d %s, want successful deletion", result.StatusCode, result.Body)
	}
	if client.input.ConditionExpression == nil || *client.input.ConditionExpression != "attribute_exists(Id)" {
		t.Fatalf("condition = %v, want attribute_exists(Id)", client.input.ConditionExpression)
	}
	if client.input.ReturnValues != types.ReturnValueAllOld {
		t.Fatalf("ReturnValues = %s, want ALL_OLD", client.input.ReturnValues)
	}
}

func TestDeletePerformanceMapsConditionalFailureTo404(t *testing.T) {
	client := &fakeDeleteClient{err: &types.ConditionalCheckFailedException{}}
	result := callDelete(t, client, "missing-performance")
	if result.StatusCode != 404 || !strings.Contains(result.Body, "missing-performance") {
		t.Fatalf("status/body = %d %s, want performance-specific 404", result.StatusCode, result.Body)
	}
}

func TestDeletePerformanceDefensivelyMapsEmptyAttributesTo404(t *testing.T) {
	result := callDelete(t, &fakeDeleteClient{}, "missing-performance")
	if result.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", result.StatusCode)
	}
}

func TestDeletePerformanceRejectsBlankIDWithoutDelete(t *testing.T) {
	client := &fakeDeleteClient{}
	result := callDelete(t, client, " ")
	if result.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", result.StatusCode)
	}
	if client.input != nil {
		t.Fatal("DeleteItem must not be called for a blank performanceId")
	}
}

func TestDeletePerformanceMapsOtherDynamoDBFailureTo500(t *testing.T) {
	result := callDelete(t, &fakeDeleteClient{err: errors.New("delete failed")}, "performance-1")
	if result.StatusCode != 500 {
		t.Fatalf("status = %d, want 500", result.StatusCode)
	}
}

func callDelete(t *testing.T, client *fakeDeleteClient, performanceID string) events.APIGatewayProxyResponse {
	t.Helper()
	handler := makeHandler(client, "PerformancesTable", nil)
	result, err := handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod:     "DELETE",
		PathParameters: map[string]string{"performanceId": performanceID},
	})
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	return result
}
