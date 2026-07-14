#!/bin/bash
NAME=$1
echo "Starting $NAME..."
eval "service $NAME start"
exit 0
