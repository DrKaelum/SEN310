# AuditionMe Final Project

AuditionMe is one AWS SAM application containing nine Go Lambda functions,
three DynamoDB tables, one SQS Standard Queue, and eight HTTP API routes. It
extends the working Lab 4 application and does not use an authorizer.

Every Lambda uses:

- Go with AWS SDK for Go v2
- `Runtime: provided.al2023`
- `Handler: bootstrap`
- `Metadata: BuildMethod: go1.x`
- `Architectures: [x86_64]`
- An independent Go module in its own `cmd/` directory

CloudFormation generates all physical table names and the queue URL. SAM passes
those values into the functions through environment variables.

## Functions and triggers

| Lambda | Trigger |
|---|---|
| CreateUserFunction | `POST /api/users` |
| GetUserProfileFunction | `GET /api/users/{userId}` |
| PostPerformanceFunction | `POST /api/performances` |
| SearchPerformancesFunction | `GET /api/performances` |
| DeletePerformanceFunction | `DELETE /api/performances/{performanceId}` |
| SignUpForAuditionFunction | `POST /api/auditions` |
| CastPerformerFunction | `POST /api/performances/{performanceId}/cast` |
| GetUserAuditionsFunction | `GET /api/users/{userId}/auditions` |
| NotifyDirectorFunction | SQS only; no API route |

The SQS workflow is:

1. Sign-up validates the request and verifies the performance.
2. It writes a pending audition.
3. It publishes one `audition_created` message.
4. SQS invokes NotifyDirector in batches of up to 10.
5. NotifyDirector processes every record and sets `notified=true`.
6. Failed records are returned with `ReportBatchItemFailures`; successful
   records are not retried.

Structured JSON lifecycle logs are produced by PostPerformance,
SignUpForAudition, CastPerformer, and NotifyDirector. Each invocation includes
`phase=start`, `phase=outcome`, and `phase=complete`.

## Run all nine unit-test suites

From the `AuditionMe` directory:

```bash
for module_dir in \
  cmd/create-user \
  cmd/get-user-profile \
  cmd/post-performance \
  cmd/search-performances \
  cmd/delete-performance \
  cmd/sign-up-for-audition \
  cmd/cast-performer \
  cmd/get-user-auditions \
  cmd/notify-director
do
  (cd "$module_dir" && go test ./...) || exit 1
done
```

Run vet the same way:

```bash
for module_dir in \
  cmd/create-user \
  cmd/get-user-profile \
  cmd/post-performance \
  cmd/search-performances \
  cmd/delete-performance \
  cmd/sign-up-for-audition \
  cmd/cast-performer \
  cmd/get-user-auditions \
  cmd/notify-director
do
  (cd "$module_dir" && go vet ./...) || exit 1
done
```

Format all deployed modules:

```bash
gofmt -w \
  cmd/create-user/*.go \
  cmd/get-user-profile/*.go \
  cmd/post-performance/*.go \
  cmd/search-performances/*.go \
  cmd/delete-performance/*.go \
  cmd/sign-up-for-audition/*.go \
  cmd/cast-performer/*.go \
  cmd/get-user-auditions/*.go \
  cmd/notify-director/*.go
```

## Validate and build

```bash
sam validate --lint
sam build --use-container
```

If dependency downloads in the build container are slow:

```bash
sam build --use-container \
  --container-env-var GOPROXY=https://proxy.golang.org,direct
```

Each of the nine built function directories under `.aws-sam/build/` must
contain an executable root-level `bootstrap`.

## Local DynamoDB and SQS

SAM Local emulates Lambda and API Gateway, but it does not create DynamoDB
tables and it does not run an SQS event-source poller. DynamoDB Local provides
the three tables. LocalStack provides only the local SQS endpoint.

Create a shared Docker network:

```bash
docker network create auditionme-local
```

Start DynamoDB Local:

```bash
docker run --detach \
  --name auditionme-dynamodb \
  --network auditionme-local \
  --publish 8000:8000 \
  amazon/dynamodb-local:latest \
  -jar DynamoDBLocal.jar -sharedDb -inMemory
```

Start LocalStack with SQS:

```bash
docker run --detach \
  --name auditionme-localstack \
  --network auditionme-local \
  --publish 4566:4566 \
  --env SERVICES=sqs \
  localstack/localstack:latest
```

Use dummy local credentials for the following commands:

```bash
export AWS_ACCESS_KEY_ID=local
export AWS_SECRET_ACCESS_KEY=local
export AWS_DEFAULT_REGION=us-east-2
```

Create the queue:

```bash
aws sqs create-queue \
  --queue-name auditionme-notifications \
  --attributes VisibilityTimeout=60 \
  --endpoint-url http://localhost:4566
```

`events/env.local.json` uses the container-visible queue URL
`http://auditionme-localstack:4566/000000000000/auditionme-notifications`.
`SQS_ENDPOINT` is blank in deployed AWS and overridden only for local work.

Create the three local tables:

```bash
aws dynamodb create-table \
  --table-name UsersTable \
  --attribute-definitions AttributeName=Id,AttributeType=S \
  --key-schema AttributeName=Id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:8000

aws dynamodb create-table \
  --table-name PerformancesTable \
  --attribute-definitions AttributeName=Id,AttributeType=S \
  --key-schema AttributeName=Id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:8000

aws dynamodb create-table \
  --table-name AuditionsTable \
  --attribute-definitions AttributeName=Id,AttributeType=S \
  --key-schema AttributeName=Id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:8000
```

## Local test data

Seed at least two items per table. This supports profile, history, cast, delete,
batch-consumer, and screenshot preparation tests.

```bash
aws dynamodb put-item \
  --table-name UsersTable \
  --item '{"Id":{"S":"performer-local-1"},"name":{"S":"Avery Stone"},"email":{"S":"avery@example.com"},"phone":{"S":"555-0101"},"role":{"S":"performer"},"created_at":{"S":"2026-07-28T12:00:00Z"}}' \
  --endpoint-url http://localhost:8000

aws dynamodb put-item \
  --table-name UsersTable \
  --item '{"Id":{"S":"director-local-1"},"name":{"S":"Dana Lee"},"email":{"S":"dana@example.com"},"phone":{"S":"555-0102"},"role":{"S":"director"},"created_at":{"S":"2026-07-28T12:01:00Z"}}' \
  --endpoint-url http://localhost:8000

aws dynamodb put-item \
  --table-name PerformancesTable \
  --item '{"Id":{"S":"performance-local-1"},"title":{"S":"Our Town"},"director":{"S":"Dana Lee"},"castingDirector":{"S":"Morgan Ray"},"venue":{"S":"Main Stage"},"performanceDates":{"L":[{"S":"2026-09-01"}]},"characters":{"L":[{"S":"Emily"},{"S":"Laura"}]},"isLive":{"BOOL":true}}' \
  --endpoint-url http://localhost:8000

aws dynamodb put-item \
  --table-name PerformancesTable \
  --item '{"Id":{"S":"performance-delete-local"},"title":{"S":"Hamlet"},"director":{"S":"Dana Lee"},"castingDirector":{"S":"Morgan Ray"},"venue":{"S":"Black Box"},"performanceDates":{"L":[{"S":"2026-10-01"}]},"characters":{"L":[{"S":"Hamlet"},{"S":"Ophelia"}]},"isLive":{"BOOL":true}}' \
  --endpoint-url http://localhost:8000

aws dynamodb put-item \
  --table-name AuditionsTable \
  --item '{"Id":{"S":"audition-local-1"},"performanceId":{"S":"performance-local-1"},"performerId":{"S":"performer-local-1"},"characterName":{"S":"Emily"},"status":{"S":"pending"}}' \
  --endpoint-url http://localhost:8000

aws dynamodb put-item \
  --table-name AuditionsTable \
  --item '{"Id":{"S":"audition-local-2"},"performanceId":{"S":"performance-local-1"},"performerId":{"S":"performer-local-1"},"characterName":{"S":"Laura"},"status":{"S":"pending"}}' \
  --endpoint-url http://localhost:8000
```

## SAM local invoke

Build first. Then invoke each function with the shared Docker network:

```bash
sam local invoke CreateUserFunction \
  --event events/create-user.json \
  --env-vars events/env.local.json \
  --docker-network auditionme-local

sam local invoke GetUserProfileFunction \
  --event events/get-user-profile.json \
  --env-vars events/env.local.json \
  --docker-network auditionme-local

sam local invoke PostPerformanceFunction \
  --event events/post-performance.json \
  --env-vars events/env.local.json \
  --docker-network auditionme-local

sam local invoke SearchPerformancesFunction \
  --event events/search-performances.json \
  --env-vars events/env.local.json \
  --docker-network auditionme-local

sam local invoke DeletePerformanceFunction \
  --event events/delete-performance.json \
  --env-vars events/env.local.json \
  --docker-network auditionme-local

sam local invoke SignUpForAuditionFunction \
  --event events/sign-up-for-audition.json \
  --env-vars events/env.local.json \
  --docker-network auditionme-local

sam local invoke CastPerformerFunction \
  --event events/cast-performer.json \
  --env-vars events/env.local.json \
  --docker-network auditionme-local

sam local invoke GetUserAuditionsFunction \
  --event events/get-user-auditions.json \
  --env-vars events/env.local.json \
  --docker-network auditionme-local

sam local invoke NotifyDirectorFunction \
  --event events/notify-director.json \
  --env-vars events/env.local.json \
  --docker-network auditionme-local
```

Additional missing-resource, duplicate-cast, invalid-message, and batch event
files are in `events/`.

## Manual local SQS handoff

Invoke SignUpForAudition, then inspect the actual local message:

```bash
aws sqs receive-message \
  --queue-url http://localhost:4566/000000000000/auditionme-notifications \
  --max-number-of-messages 10 \
  --visibility-timeout 30 \
  --endpoint-url http://localhost:4566
```

Copy each returned `MessageId` and `Body` into an `events.SQSEvent` envelope
using `events/notify-director.json` as the shape, then invoke
NotifyDirectorFunction. Finally confirm the record:

```bash
aws dynamodb get-item \
  --table-name AuditionsTable \
  --key '{"Id":{"S":"audition-local-1"}}' \
  --consistent-read \
  --endpoint-url http://localhost:8000
```

The item must contain `"notified": {"BOOL": true}`. SAM Local does not
automatically move the LocalStack message into the consumer; that manual
envelope is the local equivalent of the deployed event-source mapping.

## Local API Gateway

```bash
sam local start-api \
  --env-vars events/env.local.json \
  --docker-network auditionme-local
```

Test the eight routes at `http://localhost:3000`. No route needs an
Authorization header.

When finished:

```bash
docker stop auditionme-dynamodb auditionme-localstack
docker rm auditionme-dynamodb auditionme-localstack
docker network rm auditionme-local
```

## Deploy

```bash
sam deploy --guided
```

Use the same AWS profile and region throughout. Save the generated
configuration if desired. Use the `ApiGatewayEndpoint` CloudFormation output
as the Postman/Bruno `baseUrl`.

After deployment:

1. Confirm CloudFormation completed successfully.
2. Confirm the API has eight application methods.
3. Confirm NotifyDirector has the SQS trigger and no API trigger.
4. Run the complete live workflow.
5. Check the audition item for `notified=true`.
6. Check CloudWatch for structured JSON logs from SignUpForAudition and
   NotifyDirector.

## Complete 12-request Postman/Bruno flow

Use environment variables `baseUrl`, `directorId`, `performerId`,
`performanceId`, and `auditionId`.

1. `POST /api/users` to create a director; save `Id` as `directorId`.
2. `POST /api/users` to create a performer; save `Id` as `performerId`.
3. `POST /api/performances` to create Hamlet; save `Id` as `performanceId`.
4. `GET /api/performances`; confirm Hamlet appears.
5. `GET /api/performances?live=true`; confirm Hamlet appears.
6. `POST /api/auditions`; save `Id` as `auditionId`.
7. `POST /api/performances/{performanceId}/cast`; expect `200` and `cast`.
8. Repeat the cast; expect `409`.
9. `GET /api/users/{performerId}/auditions`; confirm `performanceTitle`.
10. `GET /api/users/{performerId}`; confirm the complete profile.
11. `DELETE /api/performances/{performanceId}`; expect `200`.
12. `GET /api/performances`; confirm Hamlet is gone.

Between requests 6 and 7, verify the asynchronous pipeline:

- The queue's `NumberOfMessagesSent` metric increased.
- NotifyDirector CloudWatch logs show receipt and `marked_notified`.
- The audition item contains DynamoDB boolean `notified=true`.

The collection should also include optional validation cases, the missing
performance sign-up, and the duplicate cast.

## Screenshot data preparation

The rubric requires at least two items in every table, while the primary
workflow creates only one performance and one audition. Before screenshots:

1. Create two users.
2. Create at least two performances.
3. Create at least two auditions.
4. Confirm both auditions become notified.
5. Capture table screenshots before deleting the showcased performance, or
   create another performance so at least two remain afterward.

## Required eight screenshots

1. CloudFormation stack and resources.
2. API Gateway resource tree with all eight application routes.
3. Users table with at least two items.
4. Performances table with at least two items.
5. Auditions table with at least two items, including `notified=true`.
6. SQS delivery evidence.
7. NotifyDirector structured CloudWatch logs.
8. Structured start/outcome/complete logs from another Lambda.

An enabled SQS consumer may remove a message before the SQS message browser can
display it. First prove the pipeline normally. Prefer durable queue metrics and
CloudWatch logs. If the instructor strictly requires a visible message body:

1. Temporarily disable the deployed event-source mapping.
2. Create one new audition.
3. Poll the queue and capture the body.
4. Immediately re-enable the mapping.
5. Verify that the audition becomes notified.
6. Confirm the mapping is enabled before final screenshots and submission.

Do not modify `template.yaml` to leave the consumer disabled.

## Two-minute demo preparation

Before recording, open:

- The live Postman/Bruno collection with the Prod `baseUrl`
- The Auditions table item view
- SQS monitoring
- NotifyDirector CloudWatch logs

Show the live API, sign-up, `notified=true`, consumer logs, successful cast,
duplicate `409`, enriched history, profile lookup, delete, and final search.
Preload tabs and variables so the asynchronous proof fits within two minutes.

## Submission archive

Build the zip from an explicit allowlist. Include:

- `template.yaml`
- `README.md`
- These nine function directories only
- `events/`
- Exported Postman collection
- Required screenshots

Exclude:

- `cmd/authorizer/`
- `cmd/list-performances/`
- `build/`
- Root `bootstrap`
- `.aws-sam/`
- Local DynamoDB data
- Credentials

The old authorizer and list-performances sources are historical only and are
not deployed by the template.

Do not delete the deployed stack until the instructor has finished grading.
After receiving the grade:

```bash
sam delete --stack-name YOUR_STACK_NAME
```

Capture the successful deletion screenshot for the final requirement.
