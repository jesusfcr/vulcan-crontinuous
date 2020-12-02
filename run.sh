#!/bin/sh

# Copyright 2020 Adevinta

export PORT=${PORT:-8080}
export PATH_STYLE=${PATH_STYLE:-false}

# Apply env variables
cat config.toml | envsubst > run.toml

./vulcan-crontinuous -c run.toml
