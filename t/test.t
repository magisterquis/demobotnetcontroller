#!/bin/ksh
#
# basic_tests.t
# Make sure our code is up-to-date and doesn't have debug things.
# By J. Stuart McMurray
# Created 20251029
# Last Modified 20251031

set -euo pipefail

. t/shmore.subr

tap_plan 22

RNUM=$RANDOM
ID=I-$RNUM
TMPD=$(mktemp -d)
OUTF=$TMPD/${ID}_out
trap 'rm -rf "$TMPD"; tap_done_testing' EXIT


# Start the server.
go run . \
        -debug \
        -dir "$TMPD" \
        -listen 127.0.0.1:0 \
        -prefix "P-$RNUM" |&
read -pr
tap_ok "$?" "Read first log line" "$0" $LINENO
tap_like \
        "$REPLY" 'Server starting' \
        "First line indicates server is starting" \
        "$0" $LINENO

# Extract useful info
SPID=$(  print -r "$REPLY" | jq --raw-output '.PID')
ADDR=$(  print -r "$REPLY" | jq --raw-output '.address')
FP=$(    print -r "$REPLY" | jq --raw-output '.fingerprint')
PREFIX=$(print -r "$REPLY" | jq --raw-output '.prefix')
DIR=$(   print -r "$REPLY" | jq --raw-output '.directory')
tap_like "$SPID"   '^\d+$'                "PID is numeric"       "$0" $LINENO
tap_like "$ADDR"   '^127.0.0.1:\d+$'      "Address looks ok"     "$0" $LINENO
tap_like "$FP"     '^[A-Za-z0-9+/]{43}=$' "Fingerprint looks ok" "$0" $LINENO
tap_is   "$PREFIX" "/P-$RNUM/"            "Prefix is correct"    "$0" $LINENO
tap_is   "$DIR"    "$TMPD"                "Directory is correct" "$0" $LINENO

# request makes a request to the server with curl, checks if the request
# succeeded, stores the next log line in $LOG, and the response in $RESPONSE.
#
# Arguments:
# $@ - Arguments to pass to curl
request() {
        # Make the request.
        curl \
                --silent \
                --insecure \
                --pinnedpubkey "sha256//$FP" \
                "https://${ADDR}${PREFIX}${ID}" \
                "$@"

        # Get the log line.
        read -pr LOG
}

# Directory should only have the TLS fingerprint.
GOT=$(ls $DIR)
WANT="_tls_fingerprint"
tap_is "$GOT" "$WANT" "Directory starts with only the fingerprint" "$0" $LINENO

# Make a request for tasking and we should sprout an empty output file.
GOT=$(request)
tap_is "$GOT" "" "Got no task without a tasking file" "$0" $LINENO
GOT=$(ls $DIR)
WANT="${ID}_out
_tls_fingerprint"
tap_is "$GOT" "$WANT" "Empty tasking created output file" "$0" $LINENO
rm "$OUTF"

# Make a request for tasking and we should get tasking.
TASK="Task: $RANDOM"
echo "$TASK" >"$TMPD/$ID"
GOT=$(request)
tap_is "$GOT" "$TASK" "Tasking correct" "$0" $LINENO
set +e
[[ -f "$TMPD/$ID" ]]
RET=$?
set -e
tap_isnt "$RET" "0" "Tasking file removed" "$0" $LINENO
rm "$OUTF"

# Make a couple of requests for output with no output.
GOT=$(request -T- <<_eof
_eof
)
tap_is "$GOT" "" "No response to empty PUT request" "$0" $LINENO
GOT=$(request --data-binary '')
tap_is "$GOT" "" "No response to empty POST request" "$0" $LINENO
GOT=$(<"$OUTF")
tap_is "$GOT" "" "No output in file from empty output requests" "$0" $LINENO

# Make a couple of requests for output with output.
PUTO="Output with PUT - $RANDOM"
GOT=$(request -T- <<_eof
$PUTO
_eof
)
tap_is "$GOT" "" "No response to PUT request with output" "$0" $LINENO
POSTO="Output with POST - $RANDOM"
GOT=$(request --data-binary "$POSTO")
tap_is "$GOT" "" "No response to POST request with output" "$0" $LINENO
GOT=$(<"$OUTF")
WANT="$PUTO
$POSTO"
tap_is "$GOT" "$WANT" "Output in file correct" "$0" $LINENO
rm "$OUTF"

# Can we get a tasking and response?
TRAND=$RANDOM
echo "echo $TRAND" >"$TMPD/$ID"
GOT=$(request | sh | request -T-)
tap_is "$?"   0  "Curl piped to sh piped to curl exited happily" "$0" $LINENO
tap_is "$GOT" "" "No response to curl piped to sh piped to curl" "$0" $LINENO
GOT=$(<"$OUTF")
tap_is "$GOT" "$TRAND" "Command output correct" "$0" $LINENO



# Stop the server.
kill "$SPID"
wait
tap_pass "Child processes exited"

# vim: ft=sh
