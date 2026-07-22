import json
from datetime import datetime, timezone


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
        "message":   "Goodbye from SAM! See you next time.",
        "timestamp": datetime.now(timezone.utc).isoformat()
    })
