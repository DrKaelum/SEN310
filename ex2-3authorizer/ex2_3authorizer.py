
import base64


def lambda_handler(event, context):
    """
    Lambda TOKEN authorizer for API Gateway.
    Validates HTTP Basic Auth credentials.
    Valid credentials: any username + password "cloudapi2026".
    """
    token = event.get("authorizationToken", "")
    method_arn = event.get("methodArn", "*")

    # ── Step 1: Check token is present and starts with "Basic " ──────────
    if not token or not token.startswith("Basic "):
        # Raising the exact string "Unauthorized" returns a 401 to the caller.
        raise Exception("Unauthorized")

    # ── Step 2: Decode the base64 credentials ────────────────────────────
    try:
        encoded = token.split(" ", 1)[1]          # everything after "Basic "
        decoded = base64.b64decode(encoded).decode("utf-8")
        username, password = decoded.split(":", 1)
    except Exception:
        raise Exception("Unauthorized")

    # ── Step 3: Validate credentials ─────────────────────────────────────
    if password != "cloudapi2026":
        # Returning a Deny policy returns 403 Forbidden to the caller.
        return _policy(username, "Deny", method_arn)

    # ── Step 4: Return Allow policy ───────────────────────────────────────
    return _policy(username, "Allow", method_arn)


def _policy(principal_id, effect, method_arn):
    return {
        "principalId": principal_id,
        "policyDocument": {
            "Version": "2012-10-17",
            "Statement": [
                {
                    "Action":   "execute-api:Invoke",
                    "Effect":   effect,
                    "Resource": method_arn
                }
            ]
        }
    }
