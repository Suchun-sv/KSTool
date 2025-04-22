#!/bin/bash

# In case of the imcompatible version of CGO, we need to build the binary with CGO_ENABLED=0
CGO_ENABLED=0 go build -o kstool main.go
