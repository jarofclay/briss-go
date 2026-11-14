#!/bin/sh
# Install Briss for you (no administrator rights needed): the program goes to
# ~/.local/bin, and an entry with the Briss icon goes to your applications menu,
# where you can also pick Briss under "Open With" for a PDF.
#
#   ./install.sh              install, or update an earlier install
#   ./install.sh --uninstall  remove it again
#
# Run it from the unpacked Briss folder of the Linux download.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
data=${XDG_DATA_HOME:-$HOME/.local/share}
bin=$HOME/.local/bin
apps=$data/applications
icons=$data/icons/hicolor/256x256/apps
id=io.github.jarofclay.briss

refresh() {
	update-desktop-database "$apps" >/dev/null 2>&1 || true
	gtk-update-icon-cache -q -t "$data/icons/hicolor" >/dev/null 2>&1 || true
}

if [ "${1:-}" = "--uninstall" ]; then
	rm -f "$bin/briss-gui" "$apps/$id.desktop" "$icons/$id.png"
	refresh
	echo "Briss is removed. Your settings stay in ~/.config/fyne/$id; delete that folder to remove them too."
	exit 0
fi

if [ ! -f "$here/briss-gui" ]; then
	echo "install.sh: briss-gui is not next to this script; run it from the unpacked Briss folder." >&2
	exit 1
fi
mkdir -p "$bin" "$apps" "$icons"
cp "$here/briss-gui" "$bin/briss-gui"
chmod 755 "$bin/briss-gui"
cp "$here/briss.png" "$icons/$id.png"
cat >"$apps/$id.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=Briss
GenericName=PDF cropper
Comment=Crop PDF pages so the text fills an e-reader, tablet or phone
Exec="$bin/briss-gui" %F
Icon=$id
Terminal=false
Categories=Office;Graphics;
MimeType=application/pdf;
Keywords=pdf;crop;scan;ebook;
StartupWMClass=Briss
EOF
refresh

echo "Briss is installed. Open it from your applications menu (search for Briss),"
echo "or run: $bin/briss-gui"
if command -v ldd >/dev/null; then
	missing=$(ldd "$bin/briss-gui" 2>/dev/null | awk '/not found/ {print $1}' | tr '\n' ' ')
	if [ -n "$missing" ]; then
		echo
		echo "Some system libraries Briss needs are missing: $missing"
		echo "On Debian or Ubuntu:  sudo apt install libgl1 libx11-6 libwayland-client0 libxkbcommon0 libxcursor1 libxrandr2 libxinerama1 libxi6 libxxf86vm1"
		echo "On Fedora:            sudo dnf install mesa-libGL libX11 libwayland-client libxkbcommon libXcursor libXrandr libXinerama libXi libXxf86vm"
	fi
fi
