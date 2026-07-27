
$ErrorActionPreference = "Stop"
$StackName = "mod05-sandbox"

Write-Host "Building..."
sam build --template-file template.yaml

Write-Host "Deploying stack '$StackName'..."
sam deploy `
  --stack-name $StackName `
  --resolve-s3 `
  --capabilities CAPABILITY_IAM `
  --no-confirm-changeset `
  --no-fail-on-empty-changeset

Write-Host ""
Write-Host "Seeding one Performance item (Id = perf-001) so the live demo has something to validate against..."

$seedItemPath = Join-Path $env:TEMP "mod05-seed-item.json"
[System.IO.File]::WriteAllText($seedItemPath, '{"Id":{"S":"perf-001"},"title":{"S":"Fall Showcase"}}')
aws dynamodb put-item `
  --table-name mod05_Performances `
  --item "file://$seedItemPath"
if ($LASTEXITCODE -ne 0) {
    Write-Host "Seeding perf-001 failed -- smoke tests below will fail too. Fix seeding before re-running." -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "Stack outputs:"
aws cloudformation describe-stacks --stack-name $StackName --query "Stacks[0].Outputs" --output table

Write-Host ""
Write-Host "Running post-deploy smoke tests (sign_up_for_audition, notify_director)..."
py tests\test_stack.py
if ($LASTEXITCODE -ne 0) {
    Write-Host ""
    Write-Host "One or more Lambdas failed smoke testing -- see output above. Stack is deployed but NOT verified." -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "Done. mod05-sign-up-for-audition and mod05-notify-director-consumer are live, wired together, and verified."
