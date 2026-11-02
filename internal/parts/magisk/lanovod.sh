#!/system/bin/sh
# LANovo resident on the Lenovo SmartClock 2 (Magisk root).
#
# The board's Root is under /data/adb, so lanovod carries this script instead
# of an init rc (parts.Carried) and keeps it current. It pins the board, makes
# the state directories, and keeps lanovod and the surface helper running in
# place of init: either one is started again whenever it exits, which is how an
# update or the panel's restart comes back. lanovod extracts the surface helper
# and its preload, so the helper starts once they are there.
#
# Running this again (as the installer does) replaces the keepers it started
# before, and displaces any other lanovod left running, such as a copy pushed
# for testing.
#
# The surface helper is libcountertop's Android 9 build; the clock runs
# Android 10, so it starts with liblanovo-shim.so preloaded (magisk/shim.c).
#
# Wi-Fi: with zygote stopped, nothing in Android drives the stock (HIDL-only)
# wpa_supplicant, so it is swapped for one with a control socket in
# /data/misc/wifi/sockets, which lanovod's radio talks to (as lanovo_wpa does
# on the other boards). Its config is seeded once from the networks the stock
# UI saved.
#
# Stopping zygote makes init start the stock boot animation, which sits above
# LANovo's layers; service.bootanim.exit=1 makes it quit as soon as it starts.
#
# Remove by deleting /data/adb/lanovod and this script; the saved state in
# /data/misc/lanovo is kept.

ROOT=/data/adb/lanovod
BOARD=helios
LOG=$ROOT/lanovod.log
KEEPERS=$ROOT/keepers.pid

[ -x "$ROOT/bin/lanovod" ] || exit 0

# Stop the keepers first, so nothing restarts what is stopped next.
[ -f "$KEEPERS" ] && kill $(cat "$KEEPERS") 2>/dev/null
rm -f "$KEEPERS"
kill $(pidof lanovod lanovo-surface) 2>/dev/null
sleep 3

/data/adb/magisk/magisk resetprop ro.lanovo.board "$BOARD" 2>/dev/null

# Each su from a shell makes Magisk start `am` to log it, which hangs forever
# with no framework; turn su logging and notifications off.
/data/adb/magisk/magisk --sqlite "UPDATE policies SET logging=0, notification=0" > /dev/null 2>&1

setprop service.bootanim.exit 1
setprop ctl.stop bootanim

mkdir -p /data/misc/lanovo /data/misc/wifi/sockets
chmod 700 /data/misc/lanovo
chmod 770 /data/misc/wifi/sockets
chown wifi:wifi /data/misc/wifi/sockets

WPA_CONF=/data/misc/wifi/lanovo_wpa.conf
if ! grep -q 'network=' "$WPA_CONF" 2>/dev/null; then
	{
		echo 'ctrl_interface=/data/misc/wifi/sockets'
		echo 'update_config=1'
		echo 'bgscan="simple:30:-65:300"'
		echo 'pmf=1'
		awk -f "$ROOT/wificonf.awk" /data/misc/wifi/WifiConfigStore.xml 2>/dev/null
	} > "$WPA_CONF.new" && mv "$WPA_CONF.new" "$WPA_CONF"
	chown wifi:wifi "$WPA_CONF"
	chmod 660 "$WPA_CONF"
fi
# The Wi-Fi HAL powers the MTK combo chip on for system_server; do it here.
if [ ! -e /sys/class/net/wlan0 ]; then
	echo 1 > /dev/wmtWifi
	i=0
	while [ ! -e /sys/class/net/wlan0 ] && [ $i -lt 20 ]; do
		sleep 1
		i=$((i+1))
	done
fi
ours=
for p in $(pidof wpa_supplicant); do
	grep -q lanovo_wpa /proc/$p/cmdline 2>/dev/null && ours=$p
done
if [ -z "$ours" ]; then
	setprop ctl.stop wpa_supplicant
	sleep 1
	rm -f /data/misc/wifi/sockets/wlan0
	ip link set wlan0 up
	/vendor/bin/hw/wpa_supplicant -B -i wlan0 -Dnl80211 -c "$WPA_CONF" < /dev/null > /dev/null 2>&1
fi

lanovod() { "$ROOT/bin/lanovod" run; }

surface() {
	while [ ! -x "$ROOT/bin/lanovo-surface" ] || [ ! -f "$ROOT/lib/liblanovo-shim.so" ]; do
		sleep 1
	done
	LD_PRELOAD="$ROOT/lib/liblanovo-shim.so" "$ROOT/bin/lanovo-surface"
}

# keep NAME: run the function NAME, and run it again whenever it exits.
keep() {
	(
		trap '' HUP
		while :; do
			# The log grows without bound; drop it at 1 MB.
			[ -f "$LOG" ] && [ "$(wc -c < "$LOG")" -gt 1048576 ] && : > "$LOG"
			"$1" >> "$LOG" 2>&1 < /dev/null
			log -t lanovod "$1 exited with $?, starting it again"
			sleep 3
		done
	) < /dev/null > /dev/null 2>&1 &
	echo $! >> "$KEEPERS"
}

keep lanovod
keep surface

# With zygote stopped, sys.boot_completed never comes, so Magisk never clears
# its bootloop counter and goes to safe mode (modules off, no service.d) on the
# third boot. Tell it the boot completed once LANovo is up.
(
	sleep 30
	[ -n "$(pidof lanovod)" ] && /data/adb/magisk/magisk --boot-complete
) < /dev/null > /dev/null 2>&1 &

# For the first two minutes: the boot animation can be started again once
# lanovod stops zygote, and netd (re)writes its policy rules, which route
# every socket through per-network tables nothing fills with zygote stopped;
# lanovod's DHCP writes the main table, so look it up ahead of them.
(
	i=0
	while [ $i -lt 60 ]; do
		[ -n "$(pidof bootanimation)" ] && setprop ctl.stop bootanim
		ip rule show | grep -q '^9000:' || ip rule add pref 9000 lookup main
		ip -6 rule show | grep -q '^9000:' || ip -6 rule add pref 9000 lookup main
		sleep 2
		i=$((i+1))
	done
) < /dev/null > /dev/null 2>&1 &
exit 0
