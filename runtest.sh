#!/usr/bin/env bash

mviedb -api-key `cat api_key` \
  -in testdata \
  -movie-out testdata/movies \
  -tv-out testdata/tv \
  -manifest testdata/manifest.json \
  -mv \
  -add-stop-words killers,dimension,internal,aac,en,sub,webrip
