import json
import boto3
from datetime import datetime, timezone
from os import getenv
from uuid import uuid4

# Same one-line table setup every DynamoDB-backed Lambda in this course has
# used since Module 3 -- no local/deployed special-casing. Locally (Ex 4-3)
# this points at a table you created by hand with the AWS CLI; deployed
# (Ex 4-4) it points at the CloudFormation-managed table. Either way, this
# code has no idea which one it's talking to, and doesn't need to.
notes_table = boto3.resource('dynamodb').Table(getenv('NOTES_TABLE', 'NotesTable'))


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


def create_note(event):
    body      = get_body(event)
    note_text = body.get("note_text", "")
    category  = body.get("category", "general")

    note = {
        "Id":         str(uuid4()),
        "note_text":  note_text,
        "category":   category,
        "created_at": datetime.now(timezone.utc).isoformat()
    }

    notes_table.put_item(Item=note)

    return response(201, note)


def get_note(event):
    note_id = event["pathParameters"]["noteId"]
    result  = notes_table.get_item(Key={"Id": note_id})

    if "Item" not in result:
        return response(404, {"message": f"Note {note_id} not found"})

    return response(200, result["Item"])


def lambda_handler(event, context):
    method = event.get("httpMethod")

    if method == "POST":
        return create_note(event)
    elif method == "GET":
        return get_note(event)

    return response(405, {"message": f"Method {method} not allowed"})
