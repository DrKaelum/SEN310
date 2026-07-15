
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
 
 
def get_body(event):
    body = event.get("body")
    return {} if body is None else json.loads(body)
 
 
def _decimal_to_float(obj):
    """JSON serializer helper — DynamoDB returns Decimal for numbers."""
    if isinstance(obj, Decimal):
        return float(obj)
    raise TypeError(f"Object of type {type(obj)} is not JSON serializable")
 
 
def lambda_handler(event, context):
    """PUT /products/{Id} — updates price and/or category."""
    path_params = event.get("pathParameters") or {}
    product_id  = path_params.get("Id", "")
    body        = get_body(event)
 
    # Verify the item exists before updating
    existing = table.get_item(Key={"Id": product_id})
    if "Item" not in existing:
        return response(404, {"message": f"Product '{product_id}' not found"})
 
    # Build the UpdateExpression dynamically for whichever fields are provided.
    # ExpressionAttributeNames aliases 'name' because 'name' is a reserved word in DynamoDB.
    update_parts = []
    expr_names   = {}
    expr_values  = {}
 
    if "price" in body:
        update_parts.append("#price = :p")
        expr_names["#price"]  = "price"
        expr_values[":p"]     = Decimal(str(body["price"]))
 
    if "category" in body:
        update_parts.append("#cat = :c")
        expr_names["#cat"]    = "category"
        expr_values[":c"]     = body["category"]
 
    if not update_parts:
        return response(400, {"message": "Provide at least one field to update: price, category"})
 
    table.update_item(
        Key={"Id": product_id},
        UpdateExpression="SET " + ", ".join(update_parts),
        ExpressionAttributeNames=expr_names,
        ExpressionAttributeValues=expr_values
    )
 
    return response(200, {"message": "Product updated", "Id": product_id})

