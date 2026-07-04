import json
from datetime import datetime, timezone
 
 
def lambda_handler(event, context):
    # Same Echo Lambda from Ex 1-1 — the code hasn't changed.
    # What's new is HOW it's deployed: inside a Docker container image.
    input_text = event.get("input_text", "")
 
    return {
        "echo":      input_text,
        "reversed":  input_text[::-1],
        "length":    len(input_text),
        "timestamp": datetime.now(timezone.utc).isoformat()
    }
 
 
def run_local_tests():
    tests = [
        {"input_text": "Hello from a container!"},
        {"input_text": "docker"},
        {},
    ]
    for event in tests:
        result = lambda_handler(event, None)
        print(f"Input:  {event}")
        print(f"Result: {result}")
        print()
 
 
if __name__ == "__main__":
    run_local_tests()
