package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

const validSecret = "AuditionMe-2026"

func handler(ctx context.Context, event events.APIGatewayCustomAuthorizerRequest) (events.APIGatewayCustomAuthorizerResponse, error) {
	token := strings.TrimSpace(event.AuthorizationToken)
	methodArn := event.MethodArn
	if methodArn == "" {
		methodArn = "*"
	}

	if token == "" {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	if !strings.HasPrefix(token, "Bearer ") {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	credential := strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
	if credential == "" {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	tokenParts := strings.Split(credential, ".")
	if len(tokenParts) != 2 || tokenParts[0] == "" || tokenParts[1] == "" {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	secret := tokenParts[0]
	role := tokenParts[1]

	if secret != validSecret {
		return policy("auditionme-user", "Deny", methodArn, ""), nil
	}

	if role != "performer" && role != "director" {
		return policy("auditionme-user", "Deny", methodArn, ""), nil
	}

	return policy(role, "Allow", methodArn, role), nil
}

func policy(principalID string, effect string, resource string, role string) events.APIGatewayCustomAuthorizerResponse {
	response := events.APIGatewayCustomAuthorizerResponse{
		PrincipalID: principalID,
		PolicyDocument: events.APIGatewayCustomAuthorizerPolicy{
			Version: "2012-10-17",
			Statement: []events.IAMPolicyStatement{
				{
					Action:   []string{"execute-api:Invoke"},
					Effect:   effect,
					Resource: []string{resource},
				},
			},
		},
	}

	if role != "" {
		response.Context = map[string]interface{}{"role": role}
	}

	return response
}

func runLocalTests() {
	tests := []struct {
		name  string
		token string
	}{
		{name: "missing token", token: ""},
		{name: "malformed no Bearer prefix", token: "AuditionMe-2026.performer"},
		{name: "malformed missing Bearer token", token: "Bearer "},
		{name: "malformed no dot", token: "Bearer AuditionMe-2026"},
		{name: "malformed too many dots", token: "Bearer AuditionMe-2026.performer.extra"},
		{name: "wrong secret", token: "Bearer WrongSecret.performer"},
		{name: "invalid role", token: "Bearer AuditionMe-2026.admin"},
		{name: "valid performer", token: "Bearer AuditionMe-2026.performer"},
		{name: "valid director", token: "Bearer AuditionMe-2026.director"},
	}

	for _, test := range tests {
		result, err := handler(context.Background(), events.APIGatewayCustomAuthorizerRequest{
			Type:               "TOKEN",
			AuthorizationToken: test.token,
			MethodArn:          "arn:aws:execute-api:us-east-1:123456789012:api-id/dev/POST/api/users",
		})

		fmt.Printf("=== %s ===\n", test.name)
		if err != nil {
			fmt.Printf("Error: %v\n\n", err)
			continue
		}

		fmt.Printf("PrincipalID: %s\n", result.PrincipalID)
		fmt.Printf("Effect: %s\n", result.PolicyDocument.Statement[0].Effect)
		fmt.Printf("Context: %v\n\n", result.Context)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "test" {
		runLocalTests()
		return
	}

	lambda.Start(handler)
}
