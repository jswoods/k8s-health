#!/bin/bash

set -xe

go get github.com/tools/godep
godep restore -v
make
