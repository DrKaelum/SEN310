# AuditionMe Lab 3 - Go Lambda DynamoDB Persistence

This project uses Go instead of the Python shown in the assignment. The instructor approved Go for the class. The Python `boto3` DynamoDB calls are implemented with the AWS SDK for Go v2. Every Lambda deployment package uses the AWS Lambda `provided.al2023` custom runtime and contains one executable named `bootstrap`.

The Lab 2 TOKEN authorizer remains unchanged and must stay attached to `POST /api/users`.

## DynamoDB Tables

Create these three tables in the same AWS Region as the Lambda functions:

| Table | Partition key | Capacity mode |
|---|---|---|
| `AuditionMe_Users_davian` | `Id` (String) | On-demand |
| `AuditionMe_Performances_davian` | `Id` (String) | On-demand |
| `AuditionMe_Auditions_davian` | `Id` (String) | On-demand |

In the DynamoDB console, choose **Create table**, enter the exact table name, set the partition key to `Id` with type String, select **On-demand**, and create the table. No sort key or secondary index is required.

The suffixed names are intentional. The assignment overview shows unsuffixed names, while the rubric refers to `AuditionMe_Performances_[yourname]`.

## Lambda Functions and Environment Variables

Create or update these functions:

| Lambda function | Source | Environment variables |
|---|---|---|
| `auditionme_create_user_davian` | `cmd/create-user` | `TABLE_NAME=AuditionMe_Users_davian` |
| `auditionme_post_performance_davian` | `cmd/post-performance` | `TABLE_NAME=AuditionMe_Performances_davian` |
| `auditionme_search_performances_davian` | `cmd/search-performances` | `TABLE_NAME=AuditionMe_Performances_davian` |
| `auditionme_sign_up_for_audition_davian` | `cmd/sign-up-for-audition` | `PERFORMANCES_TABLE_NAME=AuditionMe_Performances_davian`, `AUDITIONS_TABLE_NAME=AuditionMe_Auditions_davian` |
| Existing Lab 2 authorizer | `cmd/authorizer` | No change |

For each persistence Lambda:

- Runtime: `provided.al2023`
- Architecture: `x86_64`
- Handler: `bootstrap`
- Deployment package: the matching ZIP under `build/`

The handlers return a clear `500` configuration response if a required table-name variable is missing. They never fall back to a different table.

## Exact DynamoDB IAM Policies

Keep the normal `AWSLambdaBasicExecutionRole` permissions for CloudWatch Logs. Add only the applicable DynamoDB policy below to each Lambda execution role. Replace `REGION` and `ACCOUNT_ID` with the deployment values. Do not use `AmazonDynamoDBFullAccess`, `dynamodb:*`, or a wildcard resource.

### Create-user: Users PutItem only

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "dynamodb:PutItem",
      "Resource": "arn:aws:dynamodb:REGION:ACCOUNT_ID:table/AuditionMe_Users_davian"
    }
  ]
}
```

### Post-performance: Performances PutItem only

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "dynamodb:PutItem",
      "Resource": "arn:aws:dynamodb:REGION:ACCOUNT_ID:table/AuditionMe_Performances_davian"
    }
  ]
}
```

### Search-performances: Performances Scan only

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "dynamodb:Scan",
      "Resource": "arn:aws:dynamodb:REGION:ACCOUNT_ID:table/AuditionMe_Performances_davian"
    }
  ]
}
```

### Sign-up-for-audition: exact two-table access

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ReadPerformanceById",
      "Effect": "Allow",
      "Action": "dynamodb:GetItem",
      "Resource": "arn:aws:dynamodb:REGION:ACCOUNT_ID:table/AuditionMe_Performances_davian"
    },
    {
      "Sid": "CreateAudition",
      "Effect": "Allow",
      "Action": "dynamodb:PutItem",
      "Resource": "arn:aws:dynamodb:REGION:ACCOUNT_ID:table/AuditionMe_Auditions_davian"
    }
  ]
}
```

The sign-up role must have no other DynamoDB permissions from inline policies, customer-managed policies, or AWS-managed DynamoDB policies. It requires exactly `GetItem` on Performances and `PutItem` on Auditions.

## Local Tests

From the `AuditionMe` directory, run:

```bash
go test ./...
```

The focused fake-client tests verify the item fields, password exclusion, required `isLive`, empty-list validation, filtered and unfiltered scans, scan pagination, the missing-performance 404 path, no write after a missing performance, GetItem-before-PutItem ordering, and `status: pending`.

## Build Deployment ZIPs

Run these commands from the `AuditionMe` directory on Linux or WSL:

```bash
mkdir -p build/create-user build/post-performance build/search-performances build/sign-up-for-audition

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/create-user/bootstrap ./cmd/create-user
zip -j -FS build/create-user/create-user.zip build/create-user/bootstrap

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/post-performance/bootstrap ./cmd/post-performance
zip -j -FS build/post-performance/post-performance.zip build/post-performance/bootstrap

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/search-performances/bootstrap ./cmd/search-performances
zip -j -FS build/search-performances/search-performances.zip build/search-performances/bootstrap

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/sign-up-for-audition/bootstrap ./cmd/sign-up-for-audition
zip -j -FS build/sign-up-for-audition/sign-up-for-audition.zip build/sign-up-for-audition/bootstrap
```

Verify that every archive contains exactly one root-level `bootstrap`:

```bash
unzip -Z1 build/create-user/create-user.zip
unzip -Z1 build/post-performance/post-performance.zip
unzip -Z1 build/search-performances/search-performances.zip
unzip -Z1 build/sign-up-for-audition/sign-up-for-audition.zip
```

The old `build/list-performances/bootstrap` is a Lab 2 hardcoded artifact. Do not upload or submit it as the Lab 3 search Lambda.

## API Gateway REST API

Continue using the existing REST API and Lambda proxy integrations:

| Method and resource | Lambda integration | Authorization |
|---|---|---|
| `POST /api/users` | `auditionme_create_user_davian` | Existing TOKEN authorizer retained |
| `POST /api/performances` | `auditionme_post_performance_davian` | None required by this assignment |
| `GET /api/performances` | `auditionme_search_performances_davian` | Preserve existing public behavior |
| `POST /api/auditions` | `auditionme_sign_up_for_audition_davian` | None required by this assignment |

Keep Lambda proxy integration enabled. Add or confirm OPTIONS support for `/api/users`, `/api/performances`, and `/api/auditions`. Every Lambda-generated response includes CORS headers. For CORS on errors produced by API Gateway before a Lambda runs, also configure the REST API Gateway Responses for unauthorized, access-denied, default 4XX, and default 5XX responses.

After changing integrations or methods, deploy the REST API again to the existing `dev` stage.

## Safe Deployment Order

1. Create the three DynamoDB tables.
2. Deploy create-user, set its environment variable and narrow IAM policy, then verify the existing authorized user request.
3. Deploy post-performance, configure IAM, and add `POST /api/performances`.
4. Create at least one live and one non-live performance.
5. Deploy search-performances and configure IAM.
6. Change `GET /api/performances` from the hardcoded Lab 2 Lambda to search-performances, redeploy, and verify both GET cases.
7. Deploy sign-up-for-audition with its two environment variables and exact two-statement DynamoDB policy.
8. Add `POST /api/auditions`, redeploy, and run both sign-up tests.
9. Keep the old deployed list Lambda only until the DynamoDB-backed GET is confirmed; there is no data to migrate because its records were compiled Go literals.

## Bruno End-to-End Requests

Create a Bruno environment variable named `baseUrl` containing the deployed stage URL, for example:

```text
https://YOUR_API_ID.execute-api.YOUR_REGION.amazonaws.com/dev
```

Use `Content-Type: application/json` on each POST. The two user requests also require the existing Lab 2 authorizer header.

### 1. Create a performer

```http
POST {{baseUrl}}/api/users
Authorization: Bearer AuditionMe-2026.performer
Content-Type: application/json

{
  "name": "Avery Stone",
  "email": "avery@example.com",
  "phone": "555-0101",
  "role": "performer",
  "password": "performer-password"
}
```

Save the returned `Id` as `performerId`.

### 2. Create a director

```http
POST {{baseUrl}}/api/users
Authorization: Bearer AuditionMe-2026.director
Content-Type: application/json

{
  "name": "Morgan Lee",
  "email": "morgan@example.com",
  "phone": "555-0102",
  "role": "director",
  "password": "director-password"
}
```

### 3. Create a live performance

```http
POST {{baseUrl}}/api/performances
Content-Type: application/json

{
  "title": "The Glass Menagerie",
  "director": "Morgan Lee",
  "castingDirector": "Jordan Rivera",
  "venue": "AuditionMe Black Box Theatre",
  "performanceDates": ["2026-08-14", "2026-08-15"],
  "characters": ["Amanda", "Laura", "Tom"],
  "isLive": true
}
```

Save the returned `Id` as `performanceId`.

### 4. Search all performances

```http
GET {{baseUrl}}/api/performances
```

Expected response wrapper:

```json
{
  "performances": []
}
```

The real response contains the saved performances. DynamoDB does not guarantee their order.

### 5. Search live performances

```http
GET {{baseUrl}}/api/performances?live=true
```

Only records whose `isLive` value is `true` should appear.

### 6. Valid audition sign-up

```http
POST {{baseUrl}}/api/auditions
Content-Type: application/json

{
  "performanceId": "{{performanceId}}",
  "performerId": "{{performerId}}",
  "characterName": "Laura"
}
```

Expected status: `200`. The returned and stored audition must contain:

```json
{
  "status": "pending"
}
```

### 7. Invalid performance 404

```http
POST {{baseUrl}}/api/auditions
Content-Type: application/json

{
  "performanceId": "performance-does-not-exist",
  "performerId": "{{performerId}}",
  "characterName": "Laura"
}
```

Expected status: `404`, with a clear response such as:

```json
{
  "message": "Performance 'performance-does-not-exist' not found"
}
```

Confirm that this request did not add an Auditions record.

## Verify `status: pending`

After the valid sign-up:

1. Open DynamoDB in the AWS console.
2. Open `AuditionMe_Auditions_davian`.
3. Choose **Explore table items**.
4. Locate the item whose `Id` matches the sign-up response.
5. Confirm that `performanceId`, `performerId`, and `characterName` match the request.
6. Confirm the item contains the String attribute `status` with the exact lowercase value `pending`.

## Submission Screenshot Checklist

- `AuditionMe_Users_davian` table configuration showing `Id` as a String partition key
- Users table containing the performer and director, with no password attribute
- `AuditionMe_Performances_davian` table configuration and populated item
- Performances item showing both lists and the Boolean `isLive`
- `AuditionMe_Auditions_davian` table configuration and populated valid audition
- Audition item clearly showing `status: pending`
- Bruno valid sign-up request and `200` response
- Bruno invalid `performanceId` request and `404` response
- GET all and GET `?live=true` results
- Sign-up Lambda role showing exactly GetItem on Performances and PutItem on Auditions, with no broader DynamoDB policy
- Exported Bruno collection containing the complete end-to-end flow
