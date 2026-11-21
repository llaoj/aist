#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

identifiers='\b(analyzeCommandOutput|needsAnalysis|AnalysisComplete|ContinueAnalysis|processQuestion|RunRestored|RunWithPipe|QueryCommand|ModifyCommand|ResultSuccess|ResultFailed|ResultSkipped|ResultRejected|ToolStatusSuccess|ToolStatusFailed|ToolStatusSkipped|ToolStatusRejected|SaveMode|SaveRequest|SaveResult|SaveAppend|SaveReplace|SaveDelete|Summarizer|SummaryRequest|SummaryResult|TargetPath|CompactBlankLines|DisplayWelcome|DisplayShellCommand|EndRegion|clientConfig|clientLimits|buildClientConfig|CloudModel|CloudModelOptions|NewCloudModel)\b|AIST_CLOUD_'
qualified='\b(session\.Service|memory\.Service|memory\.Provider|memory\.Memory|config\.Provider|config\.Config|config\.Store|memory\.Store|prompt\.Provider|state\.State|llm\.Manager|llm\.ModelManager|llm\.ModelStatus|llm\.Request|llm\.Response|remote\.ServerConfig)\b'

status=0
if grep -RInE --include='*.go' "$identifiers" internal cmd; then
  status=1
fi
if grep -RInE --include='*.go' "$qualified" internal cmd; then
  status=1
fi

legacy_names='(cmd/cloud|internal/cloud|deploy/cloud/|scripts/build-cloud\.sh|release-cloud|aist-cloud)'
if grep -RInE --exclude-dir=.git --exclude-dir=pb_data --exclude-dir=dist --exclude-dir=node_modules --exclude-dir=.vitepress --exclude='*.db' --exclude=naming.md --exclude=naming-check.sh "$legacy_names" .; then
  echo "naming check failed: replace legacy cloud names with server names" >&2
  status=1
fi

if [[ "$status" -ne 0 ]]; then
  echo "naming check failed: replace deprecated identifiers with the names in docs/contributing/naming.md" >&2
  exit 1
fi

echo "naming check passed"
