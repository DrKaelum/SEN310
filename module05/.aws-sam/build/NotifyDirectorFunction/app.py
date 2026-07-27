import json
import logging
import boto3
from os import getenv

logger = logging.getLogger()
logger.setLevel(logging.INFO)

dynamodb        = boto3.resource('dynamodb')
auditions_table = dynamodb.Table(getenv('AUDITIONS_TABLE', 'mod05_Auditions'))


def lambda_handler(event, context):
    
    for record in event["Records"]:
        message = json.loads(record["body"])

        logger.info(json.dumps({
            "action":      "notify_director",
            "status":      "received",
            "event_type":  message.get("event_type"),
            "auditionId":  message.get("auditionId"),
            "performerId": message.get("performerId"),
        }))

        audition_id = message.get("auditionId")

        try:
            auditions_table.update_item(
                Key={"Id": audition_id},
                UpdateExpression="SET notified = :true",
                ExpressionAttributeValues={":true": True}
            )
            logger.info(json.dumps({
                "action":     "notify_director",
                "status":     "marked_notified",
                "auditionId": audition_id
            }))
        except Exception as e:
            logger.error(json.dumps({
                "action":     "notify_director",
                "error":      str(e),
                "auditionId": audition_id
            }))
            
            raise
