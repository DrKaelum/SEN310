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
    return response(200, {
        "your_name": getenv("INSTRUCTOR_NAME", "Instructor"),
        "message":   "Hello from SAM!",
        "timestamp": datetime.now(timezone.utc).isoformat()
    })
