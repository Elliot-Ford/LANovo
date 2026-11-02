#!/system/bin/sh
# Puts the lanovod a host pushed to /data/local/tmp in place on a Magisk board,
# with its boot script, and starts it: sh install.sh SHA256-OF-LANOVOD
#
# lanovo install and make install-lanovod run this as root, detached from adb,
# so it works the same over USB and over Wi-Fi: lanovod runs the board's
# networking (its own DHCP client), so restarting it drops the board off Wi-Fi,
# along with an adb session made over Wi-Fi, until the new lanovod is up.
#
# The pushed binary is checked against the host's digest before anything is
# touched, and every file is renamed into place and synced before lanovod is
# restarted, so an interrupted install leaves either the old files or the new
# ones, and the next boot starts whichever it finds. lanovod puts its helpers
# in place itself when it starts.

TMP=/data/local/tmp
ROOT=/data/adb/lanovod
SERVICE=/data/adb/service.d/lanovod.sh
STATE=/data/misc/lanovo

say() { log -t lanovod "install: $*"; }

want=$1
have=$(sha256sum "$TMP/lanovod" 2>/dev/null | cut -d' ' -f1)
if [ -z "$want" ] || [ "$have" != "$want" ]; then
	say "refusing: $TMP/lanovod is ${have:-missing}, wanted ${want:-a digest}"
	exit 1
fi

place() { # place SOURCE DEST MODE
	cp "$1" "$2.new" && chmod "$3" "$2.new" && mv "$2.new" "$2"
}

mkdir -p "$ROOT/bin" "$ROOT/lib" "${SERVICE%/*}" "$STATE" &&
	chmod 700 "$STATE" &&
	place "$TMP/lanovod" "$ROOT/bin/lanovod" 755 &&
	place "$TMP/lanovod.sh" "$SERVICE" 755 &&
	place "$TMP/wificonf.awk" "$ROOT/wificonf.awk" 644 || {
	say "placing the files failed; the old lanovod is still running"
	exit 1
}
if [ -f "$TMP/lanovo-name" ]; then
	place "$TMP/lanovo-name" "$STATE/name" 644
fi
sync
rm -f "$TMP/lanovod" "$TMP/lanovod.sh" "$TMP/wificonf.awk" "$TMP/lanovo-name" "$TMP/lanovo-install.sh"
say "installed $want, restarting lanovod"

exec sh "$SERVICE" </dev/null >/dev/null 2>&1
