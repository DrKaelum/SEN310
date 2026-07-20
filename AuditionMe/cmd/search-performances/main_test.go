package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type fakeScanClient struct {
	inputs  []*dynamodb.ScanInput
	outputs []*dynamodb.ScanOutput
}

func (f *fakeScanClient) Scan(_ context.Context, input *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	copyOfInput := *input
	f.inputs = append(f.inputs, &copyOfInput)
	output := f.outputs[0]
	f.outputs = f.outputs[1:]
	return output, nil
}

func TestSearchUsesUnfilteredScanNormally(t *testing.T) {
	client := &fakeScanClient{outputs: []*dynamodb.ScanOutput{{}}}
	result := callSearch(t, client, map[string]string{})
	if result.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", result.StatusCode)
	}
	if len(client.inputs) != 1 || client.inputs[0].FilterExpression != nil {
		t.Fatal("normal search must use one unfiltered Scan")
	}
	if result.Body != `{"performances":[]}` {
		t.Fatalf("body = %s, want empty performances wrapper", result.Body)
	}
}

func TestSearchAddsLiveTrueFilter(t *testing.T) {
	client := &fakeScanClient{outputs: []*dynamodb.ScanOutput{{}}}
	callSearch(t, client, map[string]string{"live": "true"})
	input := client.inputs[0]
	if input.FilterExpression == nil || *input.FilterExpression != "#live = :true" {
		t.Fatalf("filter = %v, want #live = :true", input.FilterExpression)
	}
	if input.ExpressionAttributeNames["#live"] != "isLive" {
		t.Fatal("filter must alias the isLive attribute")
	}
	value, ok := input.ExpressionAttributeValues[":true"].(*types.AttributeValueMemberBOOL)
	if !ok || !value.Value {
		t.Fatal(":true must be a DynamoDB BOOL true")
	}
}

func TestSearchHandlesPagination(t *testing.T) {
	firstItem, _ := attributevalue.MarshalMap(Performance{ID: "one", Title: "First"})
	secondItem, _ := attributevalue.MarshalMap(Performance{ID: "two", Title: "Second"})
	lastKey := map[string]types.AttributeValue{"Id": &types.AttributeValueMemberS{Value: "one"}}
	client := &fakeScanClient{outputs: []*dynamodb.ScanOutput{
		{Items: []map[string]types.AttributeValue{firstItem}, LastEvaluatedKey: lastKey},
		{Items: []map[string]types.AttributeValue{secondItem}},
	}}
	result := callSearch(t, client, nil)
	if result.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", result.StatusCode)
	}
	if len(client.inputs) != 2 {
		t.Fatalf("Scan calls = %d, want 2", len(client.inputs))
	}
	if client.inputs[1].ExclusiveStartKey["Id"].(*types.AttributeValueMemberS).Value != "one" {
		t.Fatal("second Scan did not use the first page's LastEvaluatedKey")
	}
}

func callSearch(t *testing.T, client *fakeScanClient, query map[string]string) events.APIGatewayProxyResponse {
	t.Helper()
	handler := makeHandler(client, "AuditionMe_Performances_davian", nil)
	result, err := handler(context.Background(), events.APIGatewayProxyRequest{HTTPMethod: "GET", QueryStringParameters: query})
	if err != nil {
		t.Fatalf("handler returned an error: %v", err)
	}
	return result
}
