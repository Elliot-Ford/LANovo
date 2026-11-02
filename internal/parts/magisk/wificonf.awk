# Turns Android 10's saved Wi-Fi networks (/data/misc/wifi/WifiConfigStore.xml)
# into wpa_supplicant network blocks, so LANovo's own supplicant joins the
# networks the stock UI already knew. Only WPA-PSK/SAE networks with a
# passphrase are carried; one block per SSID.

function un(s) {
	gsub(/&quot;/, "\"", s)
	gsub(/&apos;/, "'", s)
	gsub(/&lt;/, "<", s)
	gsub(/&gt;/, ">", s)
	gsub(/&amp;/, "\\&", s)
	return s
}

function val(l) {
	sub(/^[^>]*>/, "", l)
	sub(/<\/string>.*/, "", l)
	return un(l)
}

/<WifiConfiguration>/ { ssid = ""; psk = ""; hidden = 0 }
/name="SSID"/ { ssid = val($0) }
/name="PreSharedKey"/ { psk = val($0) }
/name="HiddenSSID" value="true"/ { hidden = 1 }
/<\/WifiConfiguration>/ {
	if (ssid != "" && psk != "" && !(ssid in seen)) {
		seen[ssid] = 1
		printf "network={\n\tssid=%s\n\tpsk=%s\n\tkey_mgmt=WPA-PSK SAE\n\tieee80211w=1\n%s}\n", ssid, psk, (hidden ? "\tscan_ssid=1\n" : "")
	}
}
