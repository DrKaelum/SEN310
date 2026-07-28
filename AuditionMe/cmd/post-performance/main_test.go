package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
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

func TestPostPerformanceRejectsEveryInvalidPostShape(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{"missing body", "", "Missing request body"},
		{"invalid JSON", "{", "Invalid request body"},
		{"missing title", `{"director":"D","castingDirector":"C","venue":"V","performanceDates":["2026-01-01"],"characters":["A"],"isLive":true}`, "title"},
		{"missing director", `{"title":"T","castingDirector":"C","venue":"V","performanceDates":["2026-01-01"],"characters":["A"],"isLive":true}`, "director"},
		{"missing castingDirector", `{"title":"T","director":"D","venue":"V","performanceDates":["2026-01-01"],"characters":["A"],"isLive":true}`, "castingDirector"},
		{"missing venue", `{"title":"T","director":"D","castingDirector":"C","performanceDates":["2026-01-01"],"characters":["A"],"isLive":true}`, "venue"},
		{"empty performanceDates", `{"title":"T","director":"D","castingDirector":"C","venue":"V","performanceDates":[],"characters":["A"],"isLive":true}`, "performanceDates"},
		{"blank performance date", `{"title":"T","director":"D","castingDirector":"C","venue":"V","performanceDates":[" "],"characters":["A"],"isLive":true}`, "performanceDates"},
		{"empty characters", `{"title":"T","director":"D","castingDirector":"C","venue":"V","performanceDates":["2026-01-01"],"characters":[],"isLive":true}`, "characters"},
		{"blank character", `{"title":"T","director":"D","castingDirector":"C","venue":"V","performanceDates":["2026-01-01"],"characters":[" "],"isLive":true}`, "characters"},
		{"missing isLive", `{"title":"T","director":"D","castingDirector":"C","venue":"V","performanceDates":["2026-01-01"],"characters":["A"]}`, "isLive"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			client := &fakePutItemClient{}
			handler := makeHandler(client, "PerformancesTable", nil)
			result, err := handler(context.Background(), events.APIGatewayProxyRequest{HTTPMethod: "POST", Body: testCase.body})
			if err != nil {
				t.Fatalf("handler returned an error: %v", err)
			}
			if result.StatusCode != 400 || !strings.Contains(result.Body, testCase.message) {
				t.Fatalf("status/body = %d %s, want field-specific 400 containing %q", result.StatusCode, result.Body, testCase.message)
			}
			if client.input != nil {
				t.Fatal("PutItem must not be called for invalid input")
			}
		})
	}
}

func TestPostPerformanceLogsStartOutcomeAndComplete(t *testing.T) {
	var buffer bytes.Buffer
	originalLogger := logger
	logger = slog.New(slog.NewJSONHandler(&buffer, nil))
	t.Cleanup(func() { logger = originalLogger })

	client := &fakePutItemClient{}
	handler := makeHandler(client, "PerformancesTable", nil)
	result, err := handler(context.Background(), events.APIGatewayProxyRequest{
		HTTPMethod: "POST",
		Body:       `{"title":"Our Town","director":"D","castingDirector":"C","venue":"V","performanceDates":["2026-01-01"],"characters":["Emily"],"isLive":true}`,
	})
	if err != nil || result.StatusCode != 200 {
		t.Fatalf("handler result = %d %v", result.StatusCode, err)
	}
	logs := buffer.String()
	for _, phase := range []string{`"phase":"start"`, `"phase":"outcome"`, `"phase":"complete"`} {
		if !strings.Contains(logs, phase) {
			t.Fatalf("logs missing %s: %s", phase, logs)
		}
	}
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
