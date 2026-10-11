#!/usr/bin/env bash
# Drop runner credentials before any suite binary starts.
# actions/checkout must also set persist-credentials: false so .git/config
# does not keep a token after the clone.
unset GITHUB_TOKEN GH_TOKEN ACTIONS_RUNTIME_TOKEN \
  ACTIONS_ID_TOKEN_REQUEST_TOKEN ACTIONS_ID_TOKEN_REQUEST_URL
