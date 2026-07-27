# AuditionMe Lab 4 - AWS SAM with Go

This project packages the complete AuditionMe application as one AWS SAM and
CloudFormation stack. It uses Go, AWS SDK for Go v2, the
`provided.al2023` Lambda runtime, and x86-64 executables named `bootstrap`.

The Lab 4 stack contains exactly five Lambda functions:

| Function | API route |
|---|---|
| CreateUser | `POST /api/users` |
| PostPerformance | `POST /api/performances` |
| SearchPerformances | `GET /api/performances` |
| SignUpForAudition | `POST /api/auditions` |
| CastPerformer | `POST /api/performances/{performanceId}/cast` |

It also contains three `AWS::Serverless::SimpleTable` resources. Each table has
an `Id` String partition key. CloudFormation generates the physical table
names and SAM passes them to the functions with `!Ref`.

The old Lab 2 authorizer source remains in the repository for historical
reference. It is not part of `template.yaml`, is not attached to any route,
and must not be included in the Lab 4 submission archive.

## Test all five Go modules

Each deployed function directory is an independent Go module so
`sam build --use-container` can build each `CodeUri` in isolation.

Run all five test suites from this directory:

```bash
for module_dir in \
  cmd/create-user \
  cmd/post-performance \
  cmd/search-performances \
  cmd/sign-up-for-audition \
  cmd/cast-performer
do
  (cd "$module_dir" && go test ./...) || exit 1
done
```

Run `go vet` for all five modules:

```bash
for module_dir in \
  cmd/create-user \
  cmd/post-performance \
  cmd/search-performances \
  cmd/sign-up-for-audition \
  cmd/cast-performer
do
  (cd "$module_dir" && go vet ./...) || exit 1
done
```

## Validate and build

```bash
sam validate --lint
sam build --use-container
```

If the build environment has `GOPROXY=direct` and dependency downloads are
unusually slow, use:

```bash
sam build --use-container \
  --container-env-var GOPROXY=https://proxy.golang.org,direct
```

Each built function directory under `.aws-sam/build/` should contain a
root-level `bootstrap` executable.

## Local DynamoDB

SAM Local emulates Lambda and API Gateway but does not create the three
DynamoDB tables from the template. Run DynamoDB Local on a Docker network that
the SAM containers can join:

```bash
docker network create auditionme-local
docker run --detach \
  --name auditionme-dynamodb \
  --network auditionme-local \
  --publish 8000:8000 \
  amazon/dynamodb-local:latest \
  -jar DynamoDBLocal.jar -sharedDb -inMemory
```

Create all three local tables. Run each command with the local endpoint:

```bash
AWS_ACCESS_KEY_ID=local AWS_SECRET_ACCESS_KEY=local \
aws dynamodb create-table \
  --table-name UsersTable \
  --attribute-definitions AttributeName=Id,AttributeType=S \
  --key-schema AttributeName=Id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:8000 \
  --region us-east-2

AWS_ACCESS_KEY_ID=local AWS_SECRET_ACCESS_KEY=local \
aws dynamodb create-table \
  --table-name PerformancesTable \
  --attribute-definitions AttributeName=Id,AttributeType=S \
  --key-schema AttributeName=Id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:8000 \
  --region us-east-2

AWS_ACCESS_KEY_ID=local AWS_SECRET_ACCESS_KEY=local \
aws dynamodb create-table \
  --table-name AuditionsTable \
  --attribute-definitions AttributeName=Id,AttributeType=S \
  --key-schema AttributeName=Id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:8000 \
  --region us-east-2
```

Seed the stable records referenced by the local sign-up and cast events:

```bash
AWS_ACCESS_KEY_ID=local AWS_SECRET_ACCESS_KEY=local \
aws dynamodb put-item \
  --table-name PerformancesTable \
  --item '{"Id":{"S":"performance-local-1"},"title":{"S":"Our Town"},"director":{"S":"Dana Lee"},"castingDirector":{"S":"Morgan Ray"},"venue":{"S":"Main Stage"},"performanceDates":{"L":[{"S":"2026-09-01"}]},"characters":{"L":[{"S":"Emily"},{"S":"Laura"}]},"isLive":{"BOOL":true}}' \
  --endpoint-url http://localhost:8000 \
  --region us-east-2

AWS_ACCESS_KEY_ID=local AWS_SECRET_ACCESS_KEY=local \
aws dynamodb put-item \
  --table-name AuditionsTable \
  --item '{"Id":{"S":"audition-local-1"},"performanceId":{"S":"performance-local-1"},"performerId":{"S":"performer-local-1"},"characterName":{"S":"Emily"},"status":{"S":"pending"}}' \
  --endpoint-url http://localhost:8000 \
  --region us-east-2
```

`events/env.local.json` supplies the local table names, dummy credentials, and
`DYNAMODB_ENDPOINT=http://auditionme-dynamodb:8000`. The deployed template
does not define `DYNAMODB_ENDPOINT`; when it is absent, the SDK uses AWS
DynamoDB normally.

## Local Lambda invocations

Build first, then run:

```bash
sam local invoke CreateUserFunction \
  --event events/create-user.json \
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

sam local invoke SignUpForAuditionFunction \
  --event events/sign-up-for-audition.json \
  --env-vars events/env.local.json \
  --docker-network auditionme-local

sam local invoke CastPerformerFunction \
  --event events/cast-performer.json \
  --env-vars events/env.local.json \
  --docker-network auditionme-local

sam local invoke CastPerformerFunction \
  --event events/cast-performer-duplicate.json \
  --env-vars events/env.local.json \
  --docker-network auditionme-local
```

The first cast returns `200` with `status: "cast"`. The second returns `409`.
Re-seed `audition-local-1` with `status: pending` before repeating that pair.

## Local API

```bash
sam local start-api \
  --env-vars events/env.local.json \
  --docker-network auditionme-local
```

Test the five routes at `http://localhost:3000`. No route requires an
Authorization header.

When finished with local DynamoDB:

```bash
docker stop auditionme-dynamodb
docker rm auditionme-dynamodb
docker network rm auditionme-local
```

## Deploy and test

```bash
sam deploy --guided
```

Use stack name `auditionme-lab4-davian`. Save the generated configuration when
prompted. Use the `ApiGatewayEndpoint` stack output as the Postman base URL.

The live Postman collection should include at least:

1. Create a user.
2. Create a performance.
3. Search all performances.
4. Search with `?live=true`.
5. Create a valid audition and verify `status: pending`.
6. Sign up with a nonexistent `performanceId` and verify `404`.
7. Cast the valid audition and verify `status: cast`.
8. Repeat the cast and verify `409`.

Capture the required CloudFormation, Postman, and DynamoDB screenshots before
deleting the stack.

## Delete the stack

Use the same AWS region and profile used for deployment:

```bash
sam delete --stack-name auditionme-lab4-davian
```

Capture the successful deletion output.

## Submission archive

Build the submission archive from an explicit allowlist. Include:

- `template.yaml`
- `README.md`
- the five deployed `cmd/` function folders
- `events/`
- the exported Postman collection
- required screenshots

Exclude:

- `cmd/authorizer/`
- `cmd/list-performances/`
- `build/`
- the root `bootstrap` binary
- `.aws-sam/`
- local DynamoDB data
