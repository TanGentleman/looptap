#!/bin/bash
HOST=$1
echo "Starting service on $HOST"
curl http://$HOST/health
ping -c 1 $HOST
