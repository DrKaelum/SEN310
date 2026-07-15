
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
    """DELETE /products/{Id} — deletes a product."""
    path_params = event.get("pathParameters") or {}
    product_id  = path_params.get("Id", "")
 
    # Check existence before deleting — give a meaningful 404 instead of silent success
    existing = table.get_item(Key={"Id": product_id})
    if "Item" not in existing:
        return response(404, {"message": f"Product '{product_id}' not found"})
 
    table.delete_item(Key={"Id": product_id})
    return response(200, {"message": "Product deleted", "Id": product_id})

