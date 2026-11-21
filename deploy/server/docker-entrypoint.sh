#!/bin/sh

set -e

if [ "$(id -u)" = "0" ]; then
    chown -R aist:aist /var/lib/aist
    exec su-exec aist server "$@"
fi

exec server "$@"
