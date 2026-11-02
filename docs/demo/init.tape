# Records docs/demo/init.gif from a real run: `vhs docs/demo/init.tape` from the repo root.
Output docs/demo/init.gif
Set Shell "bash"
Set FontSize 15
Set Width 1300
Set Height 520
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 22ms

Hide
Type `export PS1='$ ' && make build >/dev/null && rm -f /tmp/profile.yaml` Enter
Sleep 2s
Sleep 6s
Type `clear` Enter
Sleep 1s
Show
Type `./bin/profile-forge init --out /tmp/profile.yaml` Enter
Sleep 2s
Type `head -n 18 /tmp/profile.yaml` Enter
Sleep 4s
