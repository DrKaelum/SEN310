import json
import logging
import boto3
from datetime import datetime, timezone
from os import getenv
from uuid import uuid4


logger = logging.getLogger()
logger.setLevel(logging.INFO)

dynamodb           = boto3.resource('dynamodb')
performances_table = dynamodb.Table(getenv('PERFORMANCES_TABLE', 'mod05_Performances'))
auditions_table     = dynamodb.Table(getenv('AUDITIONS_TABLE', 'mod05_Auditions'))

sqs       = boto3.client('sqs')
QUEUE_URL = getenv('QUEUE_URL')


def response(code, body):
    return {
        "statusCode": code,
        "headers": {"Content-Type": "application/json", "Access-Control-Allow-Origin": "*"},
        "body": json.dumps(body)
    }


def get_body(event):
    b = event.get("body")
    return {} if b is None else json.loads(b)


def publish_audition_created(audition):
    
    sqs.send_message(
        QueueUrl=QUEUE_URL,
        MessageBody=json.dumps({
            "event_type":  "audition_created",
            "auditionId":  audition["Id"],
            "performerId": audition["performerId"],
            "timestamp":   datetime.now(timezone.utc).isoformat(),
        })
    )

    # The other half of notify_director/app.py's "received"/"marked_notified"
    # logs -- same auditionId ties this line to those two, across two
    # different CloudWatch log groups, into one traceable request.
    logger.info(json.dumps({
        "action":      "sign_up_for_audition",
        "status":      "queued",
        "auditionId":  audition["Id"],
        "performerId": audition["performerId"],
    }))


def lambda_handler(event, context):
    
    body = get_body(event)

    required = ["performanceId", "performerId", "characterName"]
    for field in required:
        if field not in body:
            return response(400, {"message": f"Missing required field: {field}"})

    performance_id = body["performanceId"]
    performer_id   = body["performerId"]
    character_name = body["characterName"]

    result = performances_table.get_item(Key={"Id": performance_id})
    if "Item" not in result:
        return response(404, {"message": f"Performance {performance_id} not found"})

    audition = {
        "Id":            str(uuid4()),
        "performanceId": performance_id,
        "performerId":   performer_id,
        "characterName": character_name,
        "dateCreated":   datetime.now(timezone.utc).isoformat(),
        "status":        "pending"
    }

    auditions_table.put_item(Item=audition)

    
    publish_audition_created(audition)

    return response(201, {**audition, "message": "Audition registration submitted successfully"})
