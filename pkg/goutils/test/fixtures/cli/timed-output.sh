#!/bin/bash

if [[ -z $2 ]]; then
  trap '{
    echo "interrupted";
    exit 1
  }' SIGINT
else 
  trap '{
    echo "cannot interrupt";
  }' SIGINT
fi

for i in $(seq 1 $1); do
  sleep 1
  echo "running for $i secs"
done
