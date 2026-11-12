#!/usr/bin/env bash
# Description: Sleep for a number of seconds
# Param: seconds - Amount of seconds to sleep [integer] [required]
sec=${1-"10"}
sleep $sec 
echo Done sleeping $sec seconds.

