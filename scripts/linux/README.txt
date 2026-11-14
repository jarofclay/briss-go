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
"cannot execute binary file" or "Exec format error": this download is for
    Intel and AMD processors (x86-64); on ARM, build Briss from the source
    code (see the README on GitHub).
A missing library such as libGL.so.1: install.sh names the ones missing and
    the command that installs them.

To remove Briss:  ./install.sh --uninstall

https://github.com/jarofclay/briss-go
