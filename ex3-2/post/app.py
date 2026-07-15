import json
import boto3
from decimal import Decimal
from os import getenv
from uuid import uuid4
 
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
 
 
def get_body(event):
    body = event.get("body")
    return {} if body is None else json.loads(body)
 
 
def _decimal_to_float(obj):
    """JSON serializer helper — DynamoDB returns Decimal for numbers."""
    if isinstance(obj, Decimal):
        return float(obj)
    raise TypeError(f"Object of type {type(obj)} is not JSON serializable")
 
 
def lambda_handler(event, context):
    """POST /products — creates a new product."""
    body     = get_body(event)
    name     = body.get("name", "")
    price    = body.get("price")
    category = body.get("category", "")
 
    if not name or price is None or not category:
        return response(400, {"message": "name, price, and category are required"})
 
    item = {
        "Id":       str(uuid4()),
        "name":     name,
        "price":    Decimal(str(price)),   # DynamoDB requires Decimal for numbers
        "category": category
    }
    table.put_item(Item=item)
 
    return response(201, {**item, "price": float(item["price"])})
