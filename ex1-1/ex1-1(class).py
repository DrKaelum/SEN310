import json
from datetime import datetime, timezone

def lambda_handler(event, context):
    
    input_text = event.get("input_text", "")

    return {
        "echo": input_text,
        "reversed": input_text[::-1],
        "length": len(input_text),
        "timestamp": datetime.now(timezone.utc).isoformat()
    }

def run_local_tests():
    print("hello")

    tests = [
        {"input_text": "Hello, AWS!"},
        {"input_text": "Goodbye AWS"},
        {"input_text": "x"},
        {}
    ]

    for event in tests:
        result = lambda_handler(event, None)
        print(f"Input:  {event}")
        print(f"Result:     {result}")
        print()

if __name__ == "__main__":
    run_local_tests()
