#!/bin/sh
# AppLoad entry point for the Remagic Home session host. AppLoad runs this
# inside xochitl's world, which is about to be stopped — so detach into a
# transient systemd unit (PID-1-owned, survives xochitl) and exit immediately.
HERE=$(cd "$(dirname "$0")" && pwd)
systemctl is-active --quiet remagic-home && exit 0
systemd-run --unit=remagic-home --collect /bin/bash "$HERE/home-takeover.sh"
exit 0
