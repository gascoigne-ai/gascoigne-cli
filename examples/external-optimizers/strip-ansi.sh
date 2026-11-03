#!/bin/sh
# text protocol: block text on stdin, shorter text on stdout.
# Colour codes cost tokens and tell an agent nothing.
esc=$(printf '\033')
exec sed "s/${esc}\\[[0-9;?]*[A-Za-z]//g"
