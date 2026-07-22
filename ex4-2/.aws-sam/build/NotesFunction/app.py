import json
from os import getenv


# DynamoDB write intentionally omitted — wired in Lab 4.
# This exercise focuses on SAM local testing workflow, not DynamoDB writes.
# The notes function receives a POST body, logs the parsed fields, and returns them.


def response(code, body):
    return {
        "statusCode": code,
        "headers": {
            "Content-Type": "application/json",
            "Access-Control-Allow-Origin": "*"
        },
        "body": json.dumps(body)
    }


def get_body(event):
    body = event.get("body")
    return {} if body is None else json.loads(body)


def lambda_handler(event, context):
    body      = get_body(event)
    note_text = body.get("note_text", "")
    category  = body.get("category", "general")

    # In Lab 4 this will write to DynamoDB via the NOTES_TABLE env var.
    # table_name = getenv('NOTES_TABLE')

    return response(200, {
        "received_note_text": note_text,
        "received_category":  category,
        "message":            "Note received (not yet persisted — Lab 4 adds DynamoDB)"
    })
