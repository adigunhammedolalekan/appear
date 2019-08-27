#!/bin/bash

CGO_ENABLED=0 GOOS=linux go docker -a -installsuffix cgo -ldflags '-w'