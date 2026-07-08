
import json


def response(code, body):
    return {
        "statusCode": code,
        "headers": {
            "Content-Type": "application/json",
            "Access-Control-Allow-Origin": "*"
        },
        "body": json.dumps(body)
    }


def lambda_handler(event, context):
    """
    Protected endpoint for GET /protected.
    Only reached if the authorizer Lambda returned an Allow policy.
    The authorizer's principalId flows in via requestContext.authorizer.
    """
    # API Gateway injects the authorizer's principalId here automatically.
    authorizer_context = event.get("requestContext", {}).get("authorizer", {})
    user = authorizer_context.get("principalId", "unknown")

    return response(200, {
        "message": "You are authenticated!",
        "user":    user
    })

# Note: There is no run_local_tests() here.
# To test the full authorizer flow you need API Gateway in the loop —
# you cannot simulate the requestContext.authorizer injection locally.
# Test this by deploying both Lambdas and using Postman with an
# Authorization header.
