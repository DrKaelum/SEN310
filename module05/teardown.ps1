# Deletes the entire module05 sandbox stack -- both tables, the queue,
# both Lambdas (sign_up_for_audition, notify_director), the SQS trigger,
# and every IAM role/policy SAM created for them. One command, nothing
# left behind. Run this after each session.

$ErrorActionPreference = "Stop"
sam delete --stack-name mod05-sandbox --no-prompts
