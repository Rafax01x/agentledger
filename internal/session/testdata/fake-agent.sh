#!/bin/sh
# Fake agent for tests: prints to both streams, edits one tracked file,
# creates one new file, then exits with $FAKE_EXIT (default 0).
echo "agent stdout line"
echo "agent stderr line" >&2
echo "edited by agent" >> a.txt
echo "brand new" > new.txt
exit "${FAKE_EXIT:-0}"
