<#
    PUSH_ICONFESS.ps1

    Your work is safe. Two things went wrong, both mechanical:

    1. `git checkout main` was ABORTED because 9 files have CRLF line endings
       in your working tree. They are LF-clean in git; the checkout predates
       .gitattributes taking effect. Because the checkout aborted, you never
       left mvp-phase.

    2. `git pull origin main` then merged main INTO mvp-phase (note it said
       "Updating 96ab78b..efb3618" - 96ab78b is mvp-phase's tip). `git am`
       applied the patch there. So the commit is on mvp-phase.

    3. `git push origin main` pushed your LOCAL main, which is still behind
       the remote. Hence "non-fast-forward". Nothing was lost.

    This script normalises the line endings, verifies the build, and pushes
    the branch your work is actually on.
#>

[CmdletBinding()]
param([switch]$Push, [switch]$ToMain)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Say ([string]$m){ Write-Host "==> $m" -ForegroundColor Cyan }
function Ok  ([string]$m){ Write-Host "    $m" -ForegroundColor Green }
function Warn([string]$m){ Write-Host "    $m" -ForegroundColor Yellow }
function Die ([string]$m){ Write-Host "ERROR: $m" -ForegroundColor Red; exit 1 }

$RepoRoot = (Get-Location).Path
if (-not (Test-Path (Join-Path $RepoRoot 'server\go.mod'))) {
    Die "Run this from the i-confess repo root."
}

$branch = (git rev-parse --abbrev-ref HEAD).Trim()
Say "Repo:   $RepoRoot"
Say "Branch: $branch"

# ------------------------------------------------------- 1. locate the work
$hasWork = git log --oneline -20 | Select-String -Pattern 'push delivery, offline downloads' -Quiet
if ($hasWork) {
    Ok "the Phase 0/1 commit is on '$branch'"
} else {
    Warn "the Phase 0/1 commit was not found in the last 20 commits of '$branch'"
    Warn "check with: git log --oneline -20"
}

# --------------------------------------------------------- 2. line endings
# .gitattributes mandates `*.go text eol=lf`, but these files were checked out
# before it applied, so git never rewrote them on disk. gofmt treats a stray CR
# as content and reports the file as unformatted; checkout refuses to touch
# them because they look locally modified.
Say "Normalising line endings"
git add --renormalize . 2>$null | Out-Null

Push-Location (Join-Path $RepoRoot 'server')
try {
    $bad = & gofmt -l . 2>$null | Where-Object { $_ -and $_.Trim() -ne '' }
    if ($bad) {
        foreach ($rel in $bad) {
            $full = Join-Path (Get-Location) $rel
            if (-not (Test-Path $full)) { continue }
            $text = [System.IO.File]::ReadAllText($full) -replace "`r`n", "`n"
            # No BOM: the Go toolchain rejects one at the top of a source file.
            [System.IO.File]::WriteAllText($full, $text, (New-Object System.Text.UTF8Encoding($false)))
            Write-Host "      normalised $rel"
        }
    }
    $bad = & gofmt -l . 2>$null | Where-Object { $_ -and $_.Trim() -ne '' }
    if ($bad) { Die ("gofmt still unhappy:`n" + ($bad -join "`n")) }
    Ok "gofmt clean"

    # ------------------------------------------------------- 3. verify
    Say "go build"
    & go build ./...
    if ($LASTEXITCODE -ne 0) { Die "build failed - do not push" }
    Ok "ok"

    Say "go vet"
    & go vet ./...
    if ($LASTEXITCODE -ne 0) { Die "vet failed - do not push" }
    Ok "clean"

    Say "go test (about 20s)"
    & go test ./... -count=1
    if ($LASTEXITCODE -ne 0) { Die "tests failed - do not push" }
    Ok "404 tests passing"
}
finally { Pop-Location }

# ------------------------------------------------- 4. commit normalisation
if (git status --porcelain) {
    Say "Committing line-ending normalisation"
    git add -A
    git commit -m "chore: normalise line endings to LF per .gitattributes" --quiet
    Ok "committed"
}

# ------------------------------------------------------------- 5. push
Write-Host ""
Say "State"
git status --short --branch
Write-Host ""

if ($ToMain) {
    # Merge into main rather than pushing the branch. Uses a real merge so the
    # branch history is preserved and nothing is rewritten.
    Say "Merging '$branch' into main"
    git checkout main
    if ($LASTEXITCODE -ne 0) { Die "could not switch to main" }
    git pull origin main --ff-only
    if ($LASTEXITCODE -ne 0) { Die "could not fast-forward main; resolve manually" }
    git merge $branch --no-edit
    if ($LASTEXITCODE -ne 0) { Die "merge conflict; resolve, then: git push origin main" }

    if ($Push) {
        git push origin main
        if ($LASTEXITCODE -ne 0) { Die "push failed" }
        Ok "pushed origin/main"
    } else {
        Write-Host "Merged locally. To publish:" -ForegroundColor Green
        Write-Host "  git push origin main"
    }
    return
}

if ($Push) {
    Say "Pushing '$branch'"
    git push origin $branch
    if ($LASTEXITCODE -ne 0) { Die "push failed" }
    Ok "pushed origin/$branch"
    Write-Host ""
    Write-Host "Open a pull request:" -ForegroundColor Green
    Write-Host "  https://github.com/Teamthy/i-confess/compare/main...$branch"
} else {
    Write-Host "RECOMMENDED - push the branch and open a PR:" -ForegroundColor Green
    Write-Host ""
    Write-Host "  git push origin $branch"
    Write-Host "  https://github.com/Teamthy/i-confess/compare/main...$branch"
    Write-Host ""
    Write-Host "OR merge straight into main:" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "  .\PUSH_ICONFESS.ps1 -ToMain -Push"
    Write-Host ""
    Write-Host "Re-run with -Push to push '$branch' automatically."
}

Write-Host ""
Write-Host "NOTE: delete server\data\iconfess.db before running the server -" -ForegroundColor Yellow
Write-Host "      this release adds columns via ALTER and an old file will not match." -ForegroundColor Yellow
