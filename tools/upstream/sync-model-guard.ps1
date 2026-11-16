#Requires -Version 7.0
<#
.SYNOPSIS
    Merge the latest stable ranxi2001/sub2api tag into feat/model-guard,
    verify that Model Guard wiring is still present, and run focused tests.

.DESCRIPTION
    This is the local half of the upstream sync automation. It never pushes.
    On success it leaves a merge commit on the current branch; push it or let
    .github/workflows/upstream-sync.yml open a pull request.

    Conflict handling is intentionally conservative: the merge is aborted and
    the conflicting files are printed, so the working tree is never left in a
    half-merged state by an automated run.

.PARAMETER Tag
    Upstream tag to merge (for example v2.21.6). When omitted, the newest
    stable vX.Y.Z tag is selected from the upstream remote.

.PARAMETER Branch
    Branch that must currently be checked out. Defaults to feat/model-guard.

.PARAMETER UpstreamUrl
    Upstream repository URL.

.PARAMETER SkipTests
    Skip the focused Go test run. Integrity checks still run.

.PARAMETER DryRun
    Resolve and print the sync plan without creating a branch or merging.
#>
[CmdletBinding()]
param(
    [string]$Tag,
    [string]$Branch = 'feat/model-guard',
    [string]$UpstreamUrl = 'https://github.com/ranxi2001/sub2api.git',
    [switch]$SkipTests,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$PSNativeCommandUseErrorActionPreference = $false

function Assert-GitSuccess {
    param([string]$Action)
    if ($LASTEXITCODE -ne 0) {
        throw "git $Action failed (exit code $LASTEXITCODE)"
    }
}

function Add-GitHubOutput {
    param([string]$Name, [string]$Value)
    if ($env:GITHUB_OUTPUT) {
        "$Name=$Value" | Out-File -FilePath $env:GITHUB_OUTPUT -Append -Encoding utf8
    }
}

function Get-LatestUpstreamTag {
    param([string]$Url)

    $lines = & git ls-remote --tags --refs $Url 'v*'
    Assert-GitSuccess "ls-remote --tags $Url"

    $candidates = foreach ($line in $lines) {
        $parts = $line -split '\s+'
        if ($parts.Count -lt 2) { continue }
        $ref = $parts[1] -replace '^refs/tags/', ''
        $match = [regex]::Match($ref, '^v(\d+)\.(\d+)\.(\d+)$')
        if (-not $match.Success) { continue }
        [pscustomobject]@{
            Name = $ref
            Version = [version]::Parse(('{0}.{1}.{2}' -f $match.Groups[1].Value, $match.Groups[2].Value, $match.Groups[3].Value))
        }
    }

    $latest = $candidates | Sort-Object -Property Version -Descending | Select-Object -First 1
    if (-not $latest) {
        throw "No stable vX.Y.Z tag found at $Url"
    }
    return $latest
}

function Assert-ModelGuardPresent {
    param([string]$RepoRoot)

    $requiredFiles = @(
        'backend/internal/codexeval/candy.go',
        'backend/internal/codexeval/inspection.go',
        'backend/internal/handler/admin/model_guard_wiring_test.go',
        'frontend/src/views/admin/CodexEvalView.vue'
    )
    foreach ($relativePath in $requiredFiles) {
        if (-not (Test-Path (Join-Path $RepoRoot $relativePath))) {
            throw "Model Guard file is missing: $relativePath"
        }
    }

    $requiredMarkers = @(
        @{ Path = 'backend/internal/server/routes/admin.go'; Text = 'codex-eval'; Label = 'admin routes' },
        @{ Path = 'backend/internal/server/router.go'; Text = 'InitializeCodexEval'; Label = 'router initialization' },
        @{ Path = 'backend/cmd/server/main.go'; Text = 'codexeval.StopInspection'; Label = 'shutdown hook' },
        @{ Path = 'backend/internal/web/embed_on.go'; Text = '/codex-eval'; Label = 'embedded frontend bypass' },
        @{ Path = 'frontend/src/components/layout/AppSidebar.vue'; Text = '/admin/codex-eval'; Label = 'sidebar entry' },
        @{ Path = 'frontend/src/router/index.ts'; Text = 'AdminCodexEval'; Label = 'frontend route' }
    )
    foreach ($check in $requiredMarkers) {
        $fullPath = Join-Path $RepoRoot $check.Path
        if (-not (Test-Path $fullPath)) {
            throw "Model Guard integration file is missing: $($check.Path)"
        }
        if (-not (Select-String -Path $fullPath -Pattern $check.Text -SimpleMatch -Quiet)) {
            throw "Model Guard $($check.Label) is missing from $($check.Path) (expected text: $($check.Text))"
        }
    }
}

$repoRoot = (& git rev-parse --show-toplevel).Trim()
Assert-GitSuccess 'rev-parse --show-toplevel'
Set-Location $repoRoot

$currentBranch = (& git branch --show-current).Trim()
Assert-GitSuccess 'branch --show-current'
if ($currentBranch -ne $Branch) {
    throw "Expected branch '$Branch', but '$currentBranch' is checked out. Switch branches first."
}

$dirty = @(& git status --porcelain --untracked-files=no)
Assert-GitSuccess 'status --porcelain'
if ($dirty.Count -gt 0) {
    throw "Tracked files are dirty. Commit or stash them before syncing:`n$($dirty -join "`n")"
}

$remotes = @(& git remote)
Assert-GitSuccess 'remote'
if ($remotes -contains 'upstream') {
    $configuredUrl = (& git remote get-url upstream).Trim()
    Assert-GitSuccess 'remote get-url upstream'
    if ($configuredUrl -ne $UpstreamUrl) {
        throw "Remote 'upstream' points to '$configuredUrl', expected '$UpstreamUrl'."
    }
} elseif (-not $DryRun) {
    & git remote add upstream $UpstreamUrl
    Assert-GitSuccess 'remote add upstream'
}

if ([string]::IsNullOrWhiteSpace($Tag)) {
    $latest = Get-LatestUpstreamTag -Url $UpstreamUrl
    $Tag = $latest.Name
    $version = $latest.Version
} else {
    $Tag = $Tag.Trim()
    $match = [regex]::Match($Tag, '^v(\d+)\.(\d+)\.(\d+)$')
    if (-not $match.Success) {
        throw "Tag must look like vX.Y.Z; got '$Tag'."
    }
    $version = [version]::Parse(('{0}.{1}.{2}' -f $match.Groups[1].Value, $match.Groups[2].Value, $match.Groups[3].Value))
}

Write-Host "Upstream : $UpstreamUrl"
Write-Host "Tag      : $Tag"
Write-Host "Version  : $version"
Write-Host "Branch   : $Branch"

& git fetch --no-tags $UpstreamUrl "+refs/tags/${Tag}:refs/tags/${Tag}"
Assert-GitSuccess "fetch tag $Tag"

$target = (& git rev-parse "${Tag}^{commit}").Trim()
Assert-GitSuccess "rev-parse $Tag"

Assert-ModelGuardPresent -RepoRoot $repoRoot

& git merge-base --is-ancestor $target HEAD *> $null
$ancestorExit = $LASTEXITCODE
if ($ancestorExit -eq 0) {
    Write-Host "Upstream $Tag is already contained in $Branch; nothing to do."
    Add-GitHubOutput 'tag' $Tag
    Add-GitHubOutput 'version' $version.ToString()
    Add-GitHubOutput 'changed' 'false'
    Add-GitHubOutput 'tests' 'not-run'
    exit 0
}
if ($ancestorExit -ne 1) {
    throw "git merge-base --is-ancestor failed (exit code $ancestorExit)"
}

if ($DryRun) {
    Write-Host "Dry run: would create a backup branch, merge $Tag without committing, verify Model Guard and run focused tests."
    Add-GitHubOutput 'tag' $Tag
    Add-GitHubOutput 'version' $version.ToString()
    Add-GitHubOutput 'changed' 'unknown'
    Add-GitHubOutput 'tests' 'not-run'
    exit 0
}

$oldVersion = (Get-Content (Join-Path $repoRoot 'backend/cmd/server/VERSION') -Raw).Trim()
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$backupBranch = "backup/model-guard-$oldVersion-$stamp"
& git branch $backupBranch HEAD
Assert-GitSuccess "branch $backupBranch"
Write-Host "Backup branch: $backupBranch"

& git merge --no-ff --no-commit $Tag
$mergeExit = $LASTEXITCODE
if ($mergeExit -ne 0) {
    $conflicts = @(& git diff --name-only --diff-filter=U)
    if (Test-Path (Join-Path $repoRoot '.git/MERGE_HEAD')) {
        & git merge --abort
    }
    $conflictText = if ($conflicts.Count -gt 0) { $conflicts -join "`n" } else { '(no unmerged paths; inspect git output above)' }
    throw "Merge of $Tag failed. The merge was aborted. Conflicting files:`n$conflictText`nResolve manually, then rerun the upgrade checks."
}

try {
    Assert-ModelGuardPresent -RepoRoot $repoRoot

    if (-not $SkipTests) {
        Write-Host 'Running focused Model Guard tests...'
        Push-Location (Join-Path $repoRoot 'backend')
        $oldGoMaxProcs = $env:GOMAXPROCS
        try {
            $env:GOMAXPROCS = '2'
            & go test -p 1 -tags unit ./internal/codexeval ./internal/service ./internal/repository ./internal/handler/admin -run 'Test(Codex|ModelGuard|Fingerprint|HasStandalone)' -count=1
            if ($LASTEXITCODE -ne 0) {
                throw 'Focused Model Guard tests failed.'
            }
        } finally {
            if ($null -eq $oldGoMaxProcs) {
                Remove-Item Env:GOMAXPROCS -ErrorAction SilentlyContinue
            } else {
                $env:GOMAXPROCS = $oldGoMaxProcs
            }
            Pop-Location
        }
    }

    & git commit -m "merge: upgrade to Sub2API $version"
    Assert-GitSuccess 'commit merge'
} catch {
    if (Test-Path (Join-Path $repoRoot '.git/MERGE_HEAD')) {
        & git merge --abort
    }
    throw
}

$mergeCommit = (& git rev-parse --short=9 HEAD).Trim()
Assert-GitSuccess 'rev-parse HEAD'

Add-GitHubOutput 'tag' $Tag
Add-GitHubOutput 'version' $version.ToString()
Add-GitHubOutput 'old_version' $oldVersion
Add-GitHubOutput 'changed' 'true'
Add-GitHubOutput 'merge_commit' $mergeCommit
Add-GitHubOutput 'tests' $(if ($SkipTests) { 'skipped' } else { 'passed' })

Write-Host ''
Write-Host "Merged $Tag into $Branch as $mergeCommit."
Write-Host "Next: git push origin $Branch (or let the upstream-sync workflow open a PR)."
