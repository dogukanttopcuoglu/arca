# Usage: .\ask.ps1                       -> interactive: pick a book, then ask
#        .\ask.ps1 "your question" [-Doc "book substring"]
# Example: .\ask.ps1 "leverage points" -Doc "thinking in systems"
# Loads .env into the process environment (WSL/ROOTCAUSE lesson: the vars
# must reach the process — they don't come from .env automatically) and
# runs `arc ask` against the live corpus. -Doc scopes the question to one
# indexed document (case-insensitive substring of the document ID).
param(
    [string]$Query = "",
    [string]$Doc = ""
)

Get-Content .env | ForEach-Object {
    if ($_ -match '^\s*([^#][^=]+)=(.*)$') {
        Set-Item -Path "Env:$($matches[1].Trim())" -Value $matches[2].Trim()
    }
}

if ($Query) {
    if ($Doc) {
        & .\bin\arc.exe ask -doc $Doc $Query
    } else {
        & .\bin\arc.exe ask $Query
    }
} else {
    & .\bin\arc.exe ask
}
