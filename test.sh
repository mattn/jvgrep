#!/bin/sh

set -e

go build

ls testdir/input* | while read input; do
  pattern=$(cat $(echo $input | sed -e 's/input/pattern/'))
  output=$(echo $input | sed -e 's/input/output/')
  want=$(echo $input | sed -e 's/input/want/')
  echo $input
  ./jvgrep $pattern $input > $output
  diff -u $output $want
done
