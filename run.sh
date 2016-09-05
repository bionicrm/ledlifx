#!/usr/bin/env bash

set -e

echo "Ensure 'sudo pi-blaster --gpio 2,4,3,5,7' was run on the Pi."

echo -n "Building... "
GOARCH=arm go build -o out
echo "done"

echo -n "Uploading... "
rsync out pi@10.0.0.33:/home/pi/ledlifx
rm out
echo "done"

echo "Running!"
ssh pi@10.0.0.33 "fuser -k 56700/udp && /home/pi/ledlifx"
