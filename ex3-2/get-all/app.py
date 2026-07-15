
import json
import boto3
from decimal import Decimal
from os import getenv
 
table = boto3.resource('dynamodb').Table(getenv('TABLE_NAME', 'Products'))
 
 
def response(code, body):
    return {
        "statusCode": code,
        "headers": {
            "Content-Type": "application/json",
            "Access-Control-Allow-Origin": "*"
        },
        "body": json.dumps(body, default=_decimal_to_float)
    }
 
 
def _decimal_to_float(obj):
    """JSON serializer helper — DynamoDB returns Decimal for numbers."""
    if isinstance(obj, Decimal):
        return float(obj)
    raise TypeError(f"Object of type {type(obj)} is not JSON serializable")
 
 
def lambda_handler(event, context):
    """GET /products — returns all products via table scan."""
    result = table.scan()
    return response(200, result.get("Items", []))

