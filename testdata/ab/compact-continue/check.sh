#!/bin/sh
# Solved: the answer ($1, the agent's final text) names the file marked in step 1 on line 1
# and the number of files under optimizer/ and compress/ that mention lazyre on line 2.
f=$(git diff --name-only | head -1)
n=$(grep -rl lazyre optimizer compress --include='*.go' | wc -l | tr -d ' ')
[ -n "$f" ] && [ "$(sed -n 1p "$1")" = "$f" ] && [ "$(sed -n 2p "$1")" = "$n" ]
