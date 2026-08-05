#!/bin/sh
set -e
/usr/sbin/sshd
exec nginx -g 'daemon off;'
