Briss for Linux
===============

Most file managers no longer start a program when you double-click it, so
install Briss once from a terminal. In this folder (right-click it and choose
"Open in Terminal", or cd into it) run:

    ./install.sh

Briss is then in your applications menu (search for "Briss"), and you can
choose it under "Open With" for a PDF. No administrator rights are needed: it
goes to ~/.local/bin, with a menu entry in ~/.local/share/applications.

To try it without installing, run it from this folder:

    ./briss-gui

"Permission denied": the files lost their run permission while unpacking; run
    chmod +x briss-gui install.sh
"cannot execute binary file", "Exec format error" or "qemu-x86_64-static:
    Could not open '/lib64/ld-linux-x86-64.so.2'": the download does not
    match the processor. Briss-linux-amd64 is for Intel and AMD (x86-64),
    Briss-linux-arm64 for ARM (aarch64: Raspberry Pi, ARM laptops, Linux on
    Apple silicon). "uname -m" tells which this computer has; install.sh
    checks it too.
A missing library such as libGL.so.1: install.sh names the ones missing and
    the command that installs them.

To remove Briss:  ./install.sh --uninstall

https://github.com/jarofclay/briss-go
