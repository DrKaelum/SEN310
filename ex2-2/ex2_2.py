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
 
 
def get_body(event):
    body = event.get("body")
    return {} if body is None else json.loads(body)
 
 
def lambda_handler(event, context):
    # ── Path parameter ────────────────────────────────────────────────────
    # API Gateway puts {category} into pathParameters automatically.
    # The key name must match the resource definition in API Gateway exactly.
    path_params = event.get("pathParameters") or {}
    category = path_params.get("category", "unknown")
 
    # ── Query string parameter ────────────────────────────────────────────
    # queryStringParameters can be None (not just missing) when no query
    # string is present — use "or {}" not just .get() to guard against None.
    query_params = event.get("queryStringParameters") or {}
    verbose_str  = query_params.get("verbose", "false")
    verbose      = verbose_str.lower() == "true"
 
    # ── Request body ──────────────────────────────────────────────────────
    # get_body() handles both missing body and null body safely.
    body = get_body(event)
    name  = body.get("name", "")
    value = body.get("value", "")
 
    # ── Headers ───────────────────────────────────────────────────────────
    # Header names are case-sensitive in API Gateway — match exactly.
    headers    = event.get("headers") or {}
    user_agent = headers.get("User-Agent", "unknown")
 
    result = {
        "category":   category,
        "verbose":    verbose,
        "name":       name,
        "value":      value,
        "user_agent": user_agent,
        "timestamp":  datetime.now(timezone.utc).isoformat()
    }
 
    return response(200, result)
