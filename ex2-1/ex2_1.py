import json
from datetime import datetime, timezone
from os import getenv
 
 
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
    your_name = getenv("INSTRUCTOR_NAME", "DEFAULT_MY_INSTRUCTOR_NAME")

    return response(200, {
        "message":   "Hello from the cloud!",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "your_name": your_name
    })

def run_local_tests():
    import os

    print("=== Test 1: INSTRUCTOR_NAME env var is set ===")
    os.environ["INSTRUCTOR_NAME"] = "Chris Cantera"
    event = {
        "httpMethod": "GET",
        "path": "/hello",
        "headers": {},
        "queryStringParameters": None,
        "body": None,
        "pathParameters": None,
        "requestContext": {"resourcePath": "/hello", "httpMethod": "GET"}
    }
    result = lambda_handler(event, None)
    print(f"Status: {result['statusCode']}")
    print(f"Body:   {json.loads(result['body'])}")
    print()

    print("=== Test 2: INSTRUCTOR_NAME env var is missing (uses default) ===")
    del os.environ["INSTRUCTOR_NAME"]
    result = lambda_handler(event, None)
    print(f"Status: {result['statusCode']}")
    print(f"Body:   {json.loads(result['body'])}")
    print()

if __name__ == "__main__":
    run_local_tests()


