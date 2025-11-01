#!/bin/ksh
#
# new_id.t
# Make sure the New ID log logs
# By J. Stuart McMurray
# Created 20251031
# Last Modified 20251101

set -euo pipefail

. t/shmore.subr

tap_plan 14

TMPD=$(mktemp -d)
ID=I-$RANDOM
OUTF=$TMPD/${ID}
TASKF=$TMPD/${ID}_task
PREFIX=$RANDOM
trap 'rm -rf "$TMPD"; tap_done_testing' EXIT

# Start the server going.
go run . \
        -debug \
        -dir "$TMPD" \
        -listen 127.0.0.1:0 \
        -prefix "$PREFIX" |&
read -pr
tap_ok "$?" "Read first log line" "$0" $LINENO
tap_like \
        "$REPLY" 'Server starting' \
        "First line indicates server is starting" \
        "$0" $LINENO
SPID=$(  print -r "$REPLY" | jq --raw-output '.PID')
ADDR=$(  print -r "$REPLY" | jq --raw-output '.address')
FP=$(    print -r "$REPLY" | jq --raw-output '.fingerprint')
tap_like "$SPID"   '^\d+$'                "PID is numeric"       "$0" $LINENO
tap_like "$ADDR"   '^127.0.0.1:\d+$'      "Address looks ok"     "$0" $LINENO
tap_like "$FP"     '^[A-Za-z0-9+/]{43}=$' "Fingerprint looks ok" "$0" $LINENO

# request makes a request to the server with curl stores the response in
# RESPONSE and the next log line in LOG.
#
# Arguments:
# $@ - Arguments to pass to curl
request() {
        # Make the request.
        RESPONSE=$(curl \
                --insecure \
                --pinnedpubkey "sha256//$FP" \
                --show-error \
                --silent \
                "https://${ADDR}/${PREFIX}/${ID}" \
                "$@")

        # Get the log line.
        read -pr LOG
}

# First callback should get us a New ID and tasking.
request
GOT=$(print -r "$LOG" | jq --raw-output '.msg')
WANT="New ID"
tap_is "$GOT" "$WANT" "Got New ID log line" "$0" $LINENO
GOT=$(print -r "$LOG" | jq --raw-output '.id')
WANT=$ID
tap_is "$GOT" "$WANT" "New ID has correct ID" "$0" $LINENO
read -pr LOG # Should be tasking
GOT=$(print -r "$LOG" | jq --raw-output '.msg')
WANT="No tasking"
tap_is "$GOT"      "$WANT" "Got No Tasking log line"            "$0" $LINENO
tap_is "$RESPONSE" ""      "No tasking returnd from No Tasking" "$0" $LINENO

# Second callback should get us tasking.
TASK=TASK-$RANDOM
print -r "$TASK" >"$TASKF"
request
GOT=$(print -r "$LOG" | jq --raw-output '.msg')
WANT="Tasking"
tap_is "$GOT"      "$WANT" "Got tasking log line" "$0" $LINENO
tap_is "$RESPONSE" "$TASK" "Got correct tasking"  "$0" $LINENO


# Stop the server.
kill "$SPID"
read -pr LOG
GOT=$(print -r "$LOG" | jq --raw-output '.msg')
WANT="Got signal"
tap_is "$GOT" "$WANT" "Signal catch logged" "$0" $LINENO
wait
tap_pass "Child processes exited"

# Shouldn't have any more log lines.
NLEFTOVER=0
while read -pr; do
        tap_diag "Leftover log line: $REPLY"
        : $((NLEFTOVER++))
done
tap_is $NLEFTOVER 0 "No unexpected log lines" "$0" $LINENO

# vim: ft=sh
