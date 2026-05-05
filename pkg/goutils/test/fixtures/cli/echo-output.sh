#!/bin/bash

if [[ -n $3 ]]; then
  envvar=$3
  envvalue=$(eval "echo \$$envvar")

  (>&2 echo "$2 - $envvar=$envvalue")
  echo "$1 - $envvar=$envvalue"
else
  (>&2 echo "$2 - written to stderr")
  echo "$1 - written to stdout"
fi
