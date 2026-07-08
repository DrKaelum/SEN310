# AuditionMe Lab 2 - Go Lambda REST API

This lab uses Go with the AWS Lambda `provided.al2023` custom runtime. Each Lambda is built as its own `bootstrap` binary and zipped separately.

## Local Tests

Run these from the `AuditionMe` folder:

```bash
go run ./cmd/create-user test
go run ./cmd/list-performances test
go run ./cmd/authorizer test
```

The tests cover:

- CreateUser valid request
- CreateUser missing field
- CreateUser invalid email
- ListPerformances success
- Authorizer missing token
- Authorizer malformed token
- Authorizer wrong secret
- Authorizer invalid role
- Authorizer valid performer
- Authorizer valid director

## Build Lambda Zip Files

Run these from the `AuditionMe` folder on Linux or WSL:

```bash
mkdir -p build/create-user build/list-performances build/authorizer

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/create-user/bootstrap ./cmd/create-user
cd build/create-user && zip create-user.zip bootstrap && cd ../..

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/list-performances/bootstrap ./cmd/list-performances
cd build/list-performances && zip list-performances.zip bootstrap && cd ../..

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/authorizer/bootstrap ./cmd/authorizer
cd build/authorizer && zip authorizer.zip bootstrap && cd ../..
```

Upload these zip files to Lambda:

- `build/create-user/create-user.zip`
- `build/list-performances/list-performances.zip`
- `build/authorizer/authorizer.zip`

## AWS Lambda Functions To Create

Use your name in place of `[yourname]`:

- `auditionme_create_user_[yourname]`
- `auditionme_list_performances_[yourname]`
- `auditionme_authorizer_[yourname]`

For each function:

- Runtime: `provided.al2023`
- Architecture: `x86_64` if using the commands above
- Handler: `bootstrap`

## API Gateway Setup

Create a REST API:

- Name: `auditionme-api-[yourname]`
- Stage: `dev`

Resources and methods:

- `/api/users`
  - `POST`
  - Lambda Proxy Integration enabled
  - Integrates with `auditionme_create_user_[yourname]`
  - Protected by the TOKEN authorizer

- `/api/performances`
  - `GET`
  - Lambda Proxy Integration enabled
  - Integrates with `auditionme_list_performances_[yourname]`

Authorizer:

- Type: Lambda TOKEN authorizer
- Lambda: `auditionme_authorizer_[yourname]`
- Token source: `Authorization` header
- Apply it to `POST /api/users`

## Postman Requests For Screenshots

Replace the base URL with your deployed API Gateway invoke URL:

```text
https://YOUR_API_ID.execute-api.YOUR_REGION.amazonaws.com/dev
```

1. Successful GET performances:

```text
GET /api/performances
```

2. POST users with no token, expected `401`:

```text
POST /api/users
Content-Type: application/json

{"name":"Avery Stone","email":"avery@example.com","phone":"555-0101","role":"performer","password":"performer-password"}
```

3. POST users with malformed token, expected `401`:

```text
Authorization: AuditionMe-2026.performer
```

4. POST users with wrong secret, expected `403`:

```text
Authorization: Bearer WrongSecret.performer
```

5. POST users with invalid role, expected `403`:

```text
Authorization: Bearer AuditionMe-2026.admin
```

6. POST users with valid performer token, expected `200`:

```text
Authorization: Bearer AuditionMe-2026.performer
```

7. POST users with valid director token, expected `200`:

```text
Authorization: Bearer AuditionMe-2026.director
```

Use this JSON body for the POST requests:

```json
{
  "name": "Avery Stone",
  "email": "avery@example.com",
  "phone": "555-0101",
  "role": "performer",
  "password": "performer-password"
}
```
