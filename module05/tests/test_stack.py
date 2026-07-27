import json
import sys
import time
import urllib.error
import urllib.request

import boto3

STACK_NAME = "mod05-sandbox"
POLL_ATTEMPTS = 10
POLL_INTERVAL_SECONDS = 1


def get_outputs():
    cfn = boto3.client("cloudformation")
    stacks = cfn.describe_stacks(StackName=STACK_NAME)["Stacks"]
    return {o["OutputKey"]: o["OutputValue"] for o in stacks[0]["Outputs"]}


def http(method, url, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(
        url, data=data, method=method, headers={"Content-Type": "application/json"}
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            return resp.status, json.loads(resp.read())
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read())


def test_sign_up_for_audition(api_url):
    status, body = http(
        "POST",
        api_url + "api/auditions",
        {
            "performanceId": "perf-001",
            "performerId": "smoke-test-performer",
            "characterName": "Smoke Test",
        },
    )
    ok = status == 201 and "Id" in body
    print(f"[{'PASS' if ok else 'FAIL'}] sign_up_for_audition  POST /api/auditions -> {status}")
    return ok, body.get("Id")


def test_notify_director(audition_id, table_name):
    if not audition_id:
        print("[FAIL] notify_director -- no auditionId to check (producer test failed first)")
        return False

    table = boto3.resource("dynamodb").Table(table_name)
    for attempt in range(1, POLL_ATTEMPTS + 1):
        item = table.get_item(Key={"Id": audition_id}).get("Item", {})
        if item.get("notified") is True:
            print(f"[PASS] notify_director  notified=true after {attempt}s")
            return True
        time.sleep(POLL_INTERVAL_SECONDS)

    print(f"[FAIL] notify_director  notified never became true within {POLL_ATTEMPTS}s")
    return False


def main():
    outputs = get_outputs()

    signup_ok, audition_id = test_sign_up_for_audition(outputs["ApiUrl"])
    results = [signup_ok, test_notify_director(audition_id, outputs["AuditionsTableName"])]

    failures = results.count(False)
    print()
    if failures:
        print(f"{failures} of {len(results)} Lambda(s) failed verification.")
        sys.exit(1)

    print("Both Lambdas verified end-to-end against the real deployed stack.")
    sys.exit(0)


if __name__ == "__main__":
    main()
